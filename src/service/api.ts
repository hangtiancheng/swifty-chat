/**
 * Copyright (c) 2026 hangtiancheng
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

import * as z from "zod";

import { post, postVoid, upload } from "./http";
import {
  adminGroupSchema,
  adminUserSchema,
  applySchema,
  authSchema,
  chunkVerifySchema,
  contactInfoSchema,
  friendSchema,
  groupInfoSchema,
  groupMemberSchema,
  groupSearchResultSchema,
  groupSessionSchema,
  messageSchema,
  myGroupSchema,
  tagSchema,
  uploadedAvatarSchema,
  uploadedFileSchema,
  userInfoSchema,
  userSearchResultSchema,
  userSessionSchema,
  wireList,
} from "./schemas";

export interface Credentials {
  telephone: string;
  password: string;
}

export interface ProfilePatch {
  uuid: string;
  nickname?: string;
  email?: string;
  birthday?: string;
  signature?: string;
  avatar?: string;
}

export interface GroupPatch {
  uuid: string;
  name?: string;
  notice?: string;
  add_mode?: number;
  avatar?: string;
}

export const auth = {
  login: (body: Credentials, signal?: AbortSignal) =>
    post("/login", authSchema, body, signal),
  register: (body: Credentials & { nickname: string }, signal?: AbortSignal) =>
    post("/register", authSchema, body, signal),
  /** Public endpoint: resets by telephone, no current password required. */
  updatePassword: (body: Credentials) =>
    postVoid("/user/update-password", body),
};

export const user = {
  get: (ownerId: string, signal?: AbortSignal) =>
    post("/user/get-user-info", userInfoSchema, { owner_id: ownerId }, signal),
  update: (body: ProfilePatch) => postVoid("/user/update-user-info", body),
  search: (ownerId: string, keyword: string, signal?: AbortSignal) =>
    post(
      "/user/search-user",
      wireList(userSearchResultSchema),
      { owner_id: ownerId, keyword },
      signal,
    ),
  wsLogout: (ownerId: string) =>
    postVoid("/user/ws-logout", { owner_id: ownerId }),

  listAll: (ownerId: string, signal?: AbortSignal) =>
    post(
      "/user/get-user-info-list",
      wireList(adminUserSchema),
      { owner_id: ownerId },
      signal,
    ),
  enable: (uuidList: string[]) =>
    postVoid("/user/able-users", { uuid_list: uuidList }),
  disable: (uuidList: string[]) =>
    postVoid("/user/disable-users", { uuid_list: uuidList }),
  remove: (uuidList: string[]) =>
    postVoid("/user/delete-users", { uuid_list: uuidList }),
  setAdmin: (uuidList: string[], isAdmin: number) =>
    postVoid("/user/set-admin", { uuid_list: uuidList, is_admin: isAdmin }),
};

export const contact = {
  info: (userId: string, contactId: string, signal?: AbortSignal) =>
    post(
      "/contact/get-contact-info",
      contactInfoSchema,
      { user_id: userId, contact_id: contactId },
      signal,
    ),
  friends: (ownerId: string, signal?: AbortSignal) =>
    post(
      "/contact/get-user-list",
      wireList(friendSchema),
      { owner_id: ownerId },
      signal,
    ),
  tags: (ownerId: string, signal?: AbortSignal) =>
    post(
      "/contact/get-tag-list",
      wireList(tagSchema),
      { owner_id: ownerId },
      signal,
    ),
  addTag: (ownerId: string, name: string) =>
    post("/contact/add-tag", tagSchema, { owner_id: ownerId, name }),
  /** Omit a field to leave it unchanged. */
  update: (body: {
    user_id: string;
    contact_id: string;
    note_name?: string;
    tag_id?: string;
  }) => postVoid("/contact/update-contact", body),
  joinedGroups: (ownerId: string, signal?: AbortSignal) =>
    post(
      "/contact/load-my-joined-group",
      wireList(myGroupSchema),
      { owner_id: ownerId },
      signal,
    ),
  apply: (body: {
    user_id: string;
    contact_id: string;
    contact_type: number;
    message: string;
  }) => postVoid("/contact/apply-contact", body),
  contactApplies: (userId: string, signal?: AbortSignal) =>
    post(
      "/contact/get-new-contact-list",
      wireList(applySchema),
      { user_id: userId },
      signal,
    ),
  groupApplies: (userId: string, signal?: AbortSignal) =>
    post(
      "/contact/get-add-group-list",
      wireList(applySchema),
      { user_id: userId },
      signal,
    ),
  passApply: (applyId: string) =>
    postVoid("/contact/pass-contact-apply", { apply_id: applyId }),
  refuseApply: (applyId: string) =>
    postVoid("/contact/refuse-contact-apply", { apply_id: applyId }),
  blackApply: (applyId: string) =>
    postVoid("/contact/black-apply", { apply_id: applyId }),
  remove: (userId: string, contactId: string) =>
    postVoid("/contact/delete-contact", {
      user_id: userId,
      contact_id: contactId,
    }),
  block: (userId: string, contactId: string) =>
    postVoid("/contact/black-contact", {
      user_id: userId,
      contact_id: contactId,
    }),
  unblock: (userId: string, contactId: string) =>
    postVoid("/contact/cancel-black-contact", {
      user_id: userId,
      contact_id: contactId,
    }),
};

export const session = {
  open: (sendId: string, receiveId: string, signal?: AbortSignal) =>
    post(
      "/session/open-session",
      z.string(),
      { send_id: sendId, receive_id: receiveId },
      signal,
    ),
  userList: (ownerId: string, signal?: AbortSignal) =>
    post(
      "/session/get-user-session-list",
      wireList(userSessionSchema),
      { owner_id: ownerId },
      signal,
    ),
  groupList: (ownerId: string, signal?: AbortSignal) =>
    post(
      "/session/get-group-session-list",
      wireList(groupSessionSchema),
      { owner_id: ownerId },
      signal,
    ),
  remove: (ownerId: string, sessionId: string) =>
    postVoid("/session/delete-session", {
      owner_id: ownerId,
      session_id: sessionId,
    }),
  isOpenAllowed: (sendId: string, receiveId: string) =>
    post("/session/check-open-session-allowed", z.boolean(), {
      send_id: sendId,
      receive_id: receiveId,
    }),
  markRead: (ownerId: string, receiveId: string) =>
    postVoid("/session/mark-session-read", {
      owner_id: ownerId,
      receive_id: receiveId,
    }),
};

export const message = {
  withUser: (sendId: string, receiveId: string, signal?: AbortSignal) =>
    post(
      "/message/get-message-list",
      wireList(messageSchema),
      { send_id: sendId, receive_id: receiveId },
      signal,
    ),
  withGroup: (groupId: string, signal?: AbortSignal) =>
    post(
      "/message/get-group-message-list",
      wireList(messageSchema),
      { group_id: groupId },
      signal,
    ),
  uploadAvatar: (file: File, signal?: AbortSignal) => {
    const form = new FormData();
    form.append("file", file);
    return upload("/message/upload-avatar", uploadedAvatarSchema, form, signal);
  },
  uploadFile: (file: File, signal?: AbortSignal) => {
    const form = new FormData();
    form.append("file", file);
    return upload("/message/upload-file", uploadedFileSchema, form, signal);
  },
};

export const group = {
  create: (body: {
    name: string;
    owner_id: string;
    avatar: string;
    notice?: string;
    add_mode?: number;
  }) => postVoid("/group/create-group", body),
  mine: (ownerId: string, signal?: AbortSignal) =>
    post(
      "/group/load-my-group",
      wireList(myGroupSchema),
      { owner_id: ownerId },
      signal,
    ),
  info: (groupId: string, signal?: AbortSignal) =>
    post(
      "/group/get-group-info",
      groupInfoSchema,
      { group_id: groupId },
      signal,
    ),
  update: (body: GroupPatch) => postVoid("/group/update-group-info", body),
  members: (groupId: string, signal?: AbortSignal) =>
    post(
      "/group/get-group-member-list",
      wireList(groupMemberSchema),
      { group_id: groupId },
      signal,
    ),
  removeMembers: (groupId: string, memberIds: string[]) =>
    postVoid("/group/remove-group-members", {
      group_id: groupId,
      member_ids: memberIds,
    }),
  inviteMembers: (groupId: string, memberIds: string[]) =>
    postVoid("/group/invite-group-members", {
      group_id: groupId,
      member_ids: memberIds,
    }),
  search: (ownerId: string, keyword: string, signal?: AbortSignal) =>
    post(
      "/group/search-group",
      wireList(groupSearchResultSchema),
      { owner_id: ownerId, keyword },
      signal,
    ),
  leave: (userId: string, groupId: string) =>
    postVoid("/group/leave-group", { user_id: userId, group_id: groupId }),
  dismiss: (groupId: string) =>
    postVoid("/group/dismiss-group", { group_id: groupId }),
  addMode: (groupId: string) =>
    post("/group/check-group-add-mode", z.number(), { group_id: groupId }),
  enterDirectly: (userId: string, groupId: string) =>
    postVoid("/group/enter-group-directly", {
      user_id: userId,
      group_id: groupId,
    }),

  listAll: (signal?: AbortSignal) =>
    post("/group/get-group-info-list", wireList(adminGroupSchema), {}, signal),
  removeAll: (uuidList: string[]) =>
    postVoid("/group/delete-groups", { uuid_list: uuidList }),
  setStatus: (uuidList: string[], status: number) =>
    postVoid("/group/set-groups-status", { uuid_list: uuidList, status }),
};

export const file = {
  verify: (body: { file_hash: string; chunk_cnt: number; ext_name: string }) =>
    post("/file/verify", chunkVerifySchema, body),
  uploadChunk: (
    body: { file_hash: string; ext_name: string; chunk_idx: number },
    chunk: Blob,
    signal?: AbortSignal,
  ) => {
    const form = new FormData();
    form.append("file_hash", body.file_hash);
    form.append("ext_name", body.ext_name);
    form.append("chunk_idx", String(body.chunk_idx));
    form.append("chunk", chunk);
    return upload("/file/upload-chunk", z.unknown(), form, signal);
  },
  merge: (body: { file_hash: string; ext_name: string; file_name: string }) =>
    post("/file/merge", uploadedFileSchema, body),
};

export const chatroom = {
  onlineUsers: (signal?: AbortSignal) =>
    post("/chatroom/get-online-users", wireList(z.string()), {}, signal),
  callers: (roomId: string, signal?: AbortSignal) =>
    post(
      "/chatroom/get-callers",
      wireList(z.string()),
      { room_id: roomId },
      signal,
    ),
};

export const api = {
  auth,
  user,
  contact,
  session,
  message,
  group,
  file,
  chatroom,
};
