// Copyright (c) 2026 hangtiancheng
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hangtiancheng/swifty-chat/server/internal/constant"
	"github.com/hangtiancheng/swifty-chat/server/internal/dao"
	"github.com/hangtiancheng/swifty-chat/server/internal/model"

	"github.com/hangtiancheng/swifty-chat/server/internal/util"

	"github.com/hangtiancheng/swifty.go/swifty_http"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	heartbeatInterval = 30 * time.Second
	// Three missed heartbeats mark the connection dead.
	readIdleTimeout = 90 * time.Second
)

type ChatMessageRequest struct {
	SessionId  string `json:"session_id"`
	Type       int8   `json:"type"`
	Content    string `json:"content"`
	Url        string `json:"url"`
	SendId     string `json:"send_id"`
	SendName   string `json:"send_name"`
	SendAvatar string `json:"send_avatar"`
	ReceiveId  string `json:"receive_id"`
	FileType   string `json:"file_type"`
	FileName   string `json:"file_name"`
	FileSize   string `json:"file_size"`
	AVdata     string `json:"av_data"`
}

type Client struct {
	Conn      *swifty_http.WSConn
	Uuid      string
	SendBack  chan []byte
	done      chan struct{}
	closeOnce sync.Once
}

// inbound pairs a raw frame with the authenticated uuid of the connection it
// arrived on, so the sender identity never has to be read from the payload.
type inbound struct {
	senderId string
	payload  []byte
}

// shutdown closes the connection and signals the write goroutine to exit.
// Channels are never closed, so concurrent senders cannot panic.
func (c *Client) shutdown() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.Conn.Close()
	})
}

type Server struct {
	Clients  map[string]*Client
	mutex    sync.Mutex
	Transmit chan inbound
	done     chan struct{}
	stopOnce sync.Once
}

var ChatServer *Server

func init() {
	ChatServer = &Server{
		Clients:  make(map[string]*Client),
		Transmit: make(chan inbound, constant.ChannelSize),
		done:     make(chan struct{}),
	}
}

func (s *Server) Start() {
	for {
		select {
		case <-s.done:
			s.mutex.Lock()
			clients := make([]*Client, 0, len(s.Clients))
			for _, c := range s.Clients {
				clients = append(clients, c)
			}
			s.Clients = make(map[string]*Client)
			s.mutex.Unlock()
			for _, c := range clients {
				c.shutdown()
			}
			return
		case data := <-s.Transmit:
			s.handleMessage(data)
		}
	}
}

// Stop terminates the event loop and closes every client connection.
func (s *Server) Stop() {
	s.stopOnce.Do(func() { close(s.done) })
}

func (s *Server) register(client *Client) {
	s.mutex.Lock()
	old := s.Clients[client.Uuid]
	s.Clients[client.Uuid] = client
	s.mutex.Unlock()
	if old != nil {
		old.shutdown()
	}
	log.Printf("user %s connected", client.Uuid)
	_ = client.Conn.WriteText("welcome to swifty chat")
	if old == nil {
		// Presence changed: contacts refresh their online indicators.
		BroadcastSystem(constant.NotifyOnline, client.Uuid)
		if _, err := dao.Engine.Model(&model.UserInfo{}).Where("uuid", client.Uuid).
			Update(bgCtx(), bson.M{"last_online_at": time.Now()}); err != nil {
			log.Printf("register: last_online_at update failed: %v", err)
		}
	}
}

func (s *Server) unregister(client *Client) {
	s.mutex.Lock()
	removed := false
	if cur, ok := s.Clients[client.Uuid]; ok && cur == client {
		delete(s.Clients, client.Uuid)
		removed = true
		log.Printf("user %s disconnected", client.Uuid)
	}
	s.mutex.Unlock()
	client.shutdown()
	if removed {
		// Drop out of any active call so peers close the dead streams.
		if roomId, remaining := Calls.Leave(client.Uuid); roomId != "" && len(remaining) > 0 {
			avData, _ := json.Marshal(map[string]string{
				"messageId": "PROXY", "type": "leave_call", "room_id": roomId,
			})
			rsp := MessageListItem{
				Type:      constant.MessageAudioOrVideo,
				SendId:    client.Uuid,
				ReceiveId: roomId,
				CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
				AVdata:    string(avData),
			}
			if payload, err := json.Marshal(rsp); err == nil {
				s.sendRaw(payload, remaining...)
			}
		}
		BroadcastSystem(constant.NotifyOnline, client.Uuid)
		if _, err := dao.Engine.Model(&model.UserInfo{}).Where("uuid", client.Uuid).
			Update(bgCtx(), bson.M{"last_offline_at": time.Now()}); err != nil {
			log.Printf("unregister: last_offline_at update failed: %v", err)
		}
	}
}

// resolveSender reads the authoritative display fields for a user so a client
// cannot spoof someone else's name or avatar. ok is false when the record is
// unavailable, in which case the caller keeps what the client sent.
func resolveSender(ctx context.Context, uuid string) (name, avatar string, ok bool) {
	view, err := dao.UserInfoCache.Get(ctx, uuid)
	if err != nil {
		return "", "", false
	}
	var user model.UserInfo
	if err := json.Unmarshal(view.ByteSlice(), &user); err != nil {
		return "", "", false
	}
	return user.Nickname, user.Avatar, true
}

func (s *Server) handleMessage(in inbound) {
	var req ChatMessageRequest
	if err := json.Unmarshal(in.payload, &req); err != nil {
		log.Printf("handleMessage: unmarshal failed: %v", err)
		return
	}

	// The sender is whoever owns this connection, never whoever the frame
	// claims: send_id drives persistence, routing and call-room identity.
	req.SendId = in.senderId
	if name, avatar, ok := resolveSender(bgCtx(), in.senderId); ok {
		req.SendName, req.SendAvatar = name, avatar
	}

	msg := model.Message{
		Uuid:       fmt.Sprintf("M%s", util.GetNowAndLenRandomString(11)),
		SessionId:  req.SessionId,
		Type:       req.Type,
		Content:    req.Content,
		Url:        req.Url,
		SendId:     req.SendId,
		SendName:   req.SendName,
		SendAvatar: req.SendAvatar,
		ReceiveId:  req.ReceiveId,
		FileType:   req.FileType,
		FileName:   req.FileName,
		FileSize:   req.FileSize,
		Status:     constant.MessageUnsent,
		CreatedAt:  time.Now(),
		AVdata:     req.AVdata,
	}

	if req.Type == constant.MessageAudioOrVideo {
		s.handleAVMessage(req, msg)
		return
	}

	bgCtx := bgCtx()
	if _, err := dao.Engine.Model(&msg).Insert(bgCtx, &msg); err != nil {
		log.Printf("handleMessage: insert message failed: %v", err)
		return
	}

	// Surface the conversation in both sides' session lists before fan-out.
	if msg.ReceiveId != "" && msg.ReceiveId[0] == 'U' {
		touchDirectSessions(bgCtx, msg.SendId, msg.ReceiveId)
	}

	s.broadcast(req, msg, true)
	dispatchToSwiftx(&msg)
}

// handleAVMessage interprets audio/video signaling. Call lifecycle frames
// (start/join/leave/reject) go through the call manager for busy tracking and
// room membership; sdp/candidate frames are relayed point-to-point.
func (s *Server) handleAVMessage(req ChatMessageRequest, msg model.Message) {
	var av struct {
		MessageId string `json:"messageId"`
		Type      string `json:"type"`
		Media     string `json:"media"`
		RoomId    string `json:"room_id"`
	}
	_ = json.Unmarshal([]byte(req.AVdata), &av)

	// only persist certain AV signals
	if av.MessageId == "PROXY" && (av.Type == "start_call" || av.Type == "receive_call" || av.Type == "reject_call") {
		bgCtx := bgCtx()
		if _, err := dao.Engine.Model(&msg).Insert(bgCtx, &msg); err != nil {
			log.Printf("handleMessage: insert AV message failed: %v", err)
		}
	}

	roomId := av.RoomId
	if roomId == "" {
		roomId = CallRoomId(msg.SendId, msg.ReceiveId)
	}

	switch {
	case av.MessageId == "PROXY" && av.Type == "start_call":
		s.handleStartCall(req, msg, roomId)
	case av.MessageId == "PROXY" && av.Type == "receive_call":
		// 1v1 accept: the callee joins the pair room, then the caller is told
		// to create the offer. AV signaling never echoes back to the sender.
		Calls.Join(roomId, msg.SendId)
		s.broadcast(req, msg, false)
	case av.MessageId == "PROXY" && av.Type == "join_call":
		// Group accept: existing members are notified so each one creates an
		// offer towards the newcomer.
		others := GetCallers(roomId, msg.SendId)
		Calls.Join(roomId, msg.SendId)
		s.sendAVToUsers(req, msg, others)
	case av.MessageId == "PROXY" && av.Type == "reject_call":
		// Decline: free everyone in a 1v1 pair room; group calls continue.
		if msg.ReceiveId != "" && msg.ReceiveId[0] == 'U' {
			for _, member := range Calls.Members(roomId) {
				Calls.Leave(member)
			}
			s.broadcast(req, msg, false)
		}
	case av.MessageId == "PEER_LEAVE" || (av.MessageId == "PROXY" && av.Type == "leave_call"):
		leftRoom, remaining := Calls.Leave(msg.SendId)
		notified := make(map[string]bool)
		if leftRoom != "" {
			s.sendAVToUsers(req, msg, remaining)
			for _, m := range remaining {
				notified[m] = true
			}
		}
		// A caller hanging up before the callee answered must still close the
		// callee's incoming-call popup.
		if msg.ReceiveId != "" && msg.ReceiveId[0] == 'U' && !notified[msg.ReceiveId] {
			s.broadcast(req, msg, false)
		}
	default:
		// sdp / candidate and any custom frames: plain relay.
		s.broadcast(req, msg, false)
	}
}

// handleStartCall validates availability, marks the caller busy and invites
// the callee(s). Failures are reported back to the caller as call_failed.
func (s *Server) handleStartCall(req ChatMessageRequest, msg model.Message, roomId string) {
	caller := msg.SendId
	if IsSwiftx(msg.ReceiveId) {
		s.sendCallFailed(caller, roomId, "Swiftx is a text-only assistant and cannot take calls")
		return
	}
	if Calls.IsBusy(caller) && !Calls.InRoom(roomId, caller) {
		s.sendCallFailed(caller, roomId, "you are already in a call")
		return
	}

	if msg.ReceiveId != "" && msg.ReceiveId[0] == 'U' {
		callee := msg.ReceiveId
		s.mutex.Lock()
		_, online := s.Clients[callee]
		s.mutex.Unlock()
		if !online {
			s.sendCallFailed(caller, roomId, "the other user is offline")
			return
		}
		if Calls.IsBusy(callee) {
			s.sendCallFailed(caller, roomId, "the other user is in a call")
			return
		}
		Calls.Join(roomId, caller)
		s.broadcast(req, msg, false)
		return
	}

	if msg.ReceiveId != "" && msg.ReceiveId[0] == 'G' {
		var group model.GroupInfo
		if err := dao.ActiveQuery(&group).Where("uuid", msg.ReceiveId).First(bgCtx(), &group); err != nil {
			s.sendCallFailed(caller, roomId, "group not found")
			return
		}
		alreadyActive := len(Calls.Members(roomId)) > 0
		var candidates []string
		isMember := false
		s.mutex.Lock()
		for _, member := range group.Members {
			if member == caller {
				isMember = true
				continue
			}
			if _, online := s.Clients[member]; !online {
				continue
			}
			if Calls.IsBusy(member) {
				continue
			}
			candidates = append(candidates, member)
		}
		s.mutex.Unlock()
		if !isMember {
			s.sendCallFailed(caller, roomId, "you are not a member of this group")
			return
		}
		if len(candidates) == 0 && !alreadyActive {
			s.sendCallFailed(caller, roomId, "no one is available for the call")
			return
		}
		Calls.Join(roomId, caller)
		s.sendAVToUsers(req, msg, candidates)
	}
}

// sendAVToUsers relays an AV frame to an explicit target list.
func (s *Server) sendAVToUsers(req ChatMessageRequest, msg model.Message, targets []string) {
	if len(targets) == 0 {
		return
	}
	rsp := MessageListItem{
		Uuid:   msg.Uuid,
		SendId: msg.SendId, SendName: msg.SendName, SendAvatar: req.SendAvatar,
		ReceiveId: msg.ReceiveId, Type: msg.Type, Content: msg.Content,
		Url: msg.Url, FileSize: msg.FileSize, FileName: msg.FileName,
		FileType: msg.FileType, CreatedAt: msg.CreatedAt.Format("2006-01-02 15:04:05"),
		AVdata: msg.AVdata,
	}
	payload, err := json.Marshal(rsp)
	if err != nil {
		log.Printf("sendAVToUsers: marshal failed: %v", err)
		return
	}
	s.sendRaw(payload, targets...)
}

// sendCallFailed reports a failed call attempt back to the caller.
func (s *Server) sendCallFailed(uuid, roomId, reason string) {
	avData, _ := json.Marshal(map[string]string{
		"messageId": "PROXY",
		"type":      "call_failed",
		"room_id":   roomId,
		"reason":    reason,
	})
	rsp := MessageListItem{
		Type:      constant.MessageAudioOrVideo,
		SendId:    "SYSTEM",
		ReceiveId: uuid,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
		AVdata:    string(avData),
	}
	payload, err := json.Marshal(rsp)
	if err != nil {
		return
	}
	s.sendRaw(payload, uuid)
}

// sendRaw delivers a payload to the given clients without blocking.
func (s *Server) sendRaw(payload []byte, targets ...string) {
	s.mutex.Lock()
	clients := make([]*Client, 0, len(targets))
	for _, id := range targets {
		if c, ok := s.Clients[id]; ok {
			clients = append(clients, c)
		}
	}
	s.mutex.Unlock()
	for _, c := range clients {
		select {
		case c.SendBack <- payload:
		default:
			log.Printf("sendRaw: client %s buffer full, payload dropped", c.Uuid)
		}
	}
}

// PushSystem sends a system notification (MessageSystem frame) to specific
// users; the content carries the topic the client should refresh.
func PushSystem(topic string, uuids ...string) {
	if len(uuids) == 0 {
		return
	}
	rsp := MessageListItem{
		Type:      constant.MessageSystem,
		SendId:    "SYSTEM",
		Content:   topic,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	payload, err := json.Marshal(rsp)
	if err != nil {
		return
	}
	ChatServer.sendRaw(payload, uuids...)
}

// BroadcastSystem sends a system notification to every online user except the
// excluded one (typically the originator).
func BroadcastSystem(topic string, exclude string) {
	ChatServer.mutex.Lock()
	targets := make([]string, 0, len(ChatServer.Clients))
	for uuid := range ChatServer.Clients {
		if uuid != exclude {
			targets = append(targets, uuid)
		}
	}
	ChatServer.mutex.Unlock()
	PushSystem(topic, targets...)
}

// broadcast delivers the message to its receiver(s). Sends are non-blocking:
// a slow client's full buffer drops the payload instead of stalling the
// event loop while holding the server mutex.
func (s *Server) broadcast(req ChatMessageRequest, msg model.Message, echoToSender bool) {
	if msg.ReceiveId == "" {
		log.Println("broadcast: empty receive_id, skip")
		return
	}
	rsp := MessageListItem{
		Uuid:   msg.Uuid,
		SendId: msg.SendId, SendName: msg.SendName, SendAvatar: req.SendAvatar,
		ReceiveId: msg.ReceiveId, Type: msg.Type, Content: msg.Content,
		Url: msg.Url, FileSize: msg.FileSize, FileName: msg.FileName,
		FileType: msg.FileType, CreatedAt: msg.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	if msg.Type == constant.MessageAudioOrVideo {
		rsp.AVdata = msg.AVdata
	}
	jsonMsg, err := json.Marshal(rsp)
	if err != nil {
		log.Printf("broadcast: marshal failed: %v", err)
		return
	}

	var targets []string
	if msg.ReceiveId[0] == 'U' {
		targets = append(targets, msg.ReceiveId)
		if echoToSender && msg.SendId != msg.ReceiveId {
			targets = append(targets, msg.SendId)
		}
	} else if msg.ReceiveId[0] == 'G' {
		var group model.GroupInfo
		if err := dao.ActiveQuery(&group).Where("uuid", msg.ReceiveId).First(bgCtx(), &group); err != nil {
			log.Printf("broadcast: load group %s failed: %v", msg.ReceiveId, err)
			return
		}
		if msg.Type != constant.MessageAudioOrVideo {
			touchGroupSessions(bgCtx(), &group)
		}
		for _, member := range group.Members {
			if !echoToSender && member == msg.SendId {
				continue
			}
			targets = append(targets, member)
		}
	} else {
		return
	}

	s.mutex.Lock()
	clients := make([]*Client, 0, len(targets))
	for _, id := range targets {
		if c, ok := s.Clients[id]; ok {
			clients = append(clients, c)
		}
	}
	s.mutex.Unlock()

	delivered := 0
	for _, c := range clients {
		select {
		case c.SendBack <- jsonMsg:
			delivered++
		default:
			log.Printf("broadcast: client %s buffer full, message %s dropped", c.Uuid, msg.Uuid)
		}
	}
	if delivered > 0 && msg.Type != constant.MessageAudioOrVideo {
		s.markSent(msg.Uuid)
	}
}

func (s *Server) markSent(uuid string) {
	bgCtx := bgCtx()
	if _, err := dao.Engine.Model(&model.Message{}).Where("uuid", uuid).Update(bgCtx, bson.M{
		"status":  constant.MessageSent,
		"send_at": time.Now(),
	}); err != nil {
		log.Printf("markSent %s failed: %v", uuid, err)
	}
}

func NewClientInit(ws *swifty_http.WSConn, clientId string) {
	client := &Client{
		Conn:     ws,
		Uuid:     clientId,
		SendBack: make(chan []byte, constant.ChannelSize),
		done:     make(chan struct{}),
	}
	ChatServer.register(client)
	go clientWrite(client)
	go clientRead(client)
}

func ClientLogout(clientId string) (string, int) {
	ChatServer.mutex.Lock()
	client, ok := ChatServer.Clients[clientId]
	ChatServer.mutex.Unlock()
	if ok {
		ChatServer.unregister(client)
	}
	return "logout successful", 0
}

// clientRead runs the event-driven read loop with heartbeat-based dead
// connection detection: the server pings every heartbeatInterval and the
// read deadline is refreshed on every pong or message.
func clientRead(c *Client) {
	defer ChatServer.unregister(c)

	stopHeartbeat := c.Conn.Heartbeat(heartbeatInterval)
	defer stopHeartbeat()

	refresh := func() { _ = c.Conn.SetReadDeadline(time.Now().Add(readIdleTimeout)) }
	refresh()
	c.Conn.OnPong(func([]byte) { refresh() })
	c.Conn.OnError(func(err error) {
		log.Printf("ws read error for %s: %v", c.Uuid, err)
	})
	c.Conn.OnMessage(func(messageType int, data []byte) {
		refresh()
		if messageType != swifty_http.TextMessage {
			return
		}
		payload := make([]byte, len(data))
		copy(payload, data)
		select {
		case ChatServer.Transmit <- inbound{senderId: c.Uuid, payload: payload}:
		default:
			log.Printf("transmit channel full, message from %s rejected", c.Uuid)
			_ = c.Conn.WriteText(`{"type":-1,"send_id":"","receive_id":"","content":"message send failed, please retry"}`)
		}
	})
	c.Conn.Listen()
}

func clientWrite(c *Client) {
	for {
		select {
		case msg := <-c.SendBack:
			if err := c.Conn.WriteMessage(swifty_http.TextMessage, msg); err != nil {
				log.Printf("ws write error for %s: %v", c.Uuid, err)
				ChatServer.unregister(c)
				return
			}
		case <-c.done:
			return
		}
	}
}

func GetOnlineUserList() []string {
	ChatServer.mutex.Lock()
	defer ChatServer.mutex.Unlock()
	users := make([]string, 0, len(ChatServer.Clients))
	for uuid := range ChatServer.Clients {
		users = append(users, uuid)
	}
	return users
}

func IsOnline(uuid string) bool {
	ChatServer.mutex.Lock()
	defer ChatServer.mutex.Unlock()
	_, ok := ChatServer.Clients[uuid]
	return ok
}

func bgCtx() context.Context {
	return context.Background()
}
