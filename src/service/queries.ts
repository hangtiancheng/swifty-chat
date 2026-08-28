import { queryOptions } from "@tanstack/react-query";
import { orderBy } from "es-toolkit";

import { chatroom, contact, group, message, session, user } from "./api";
import {
  isGroupId,
  toChatSession,
  type ChatSession,
  type UserSession,
  type GroupSession,
} from "./schemas";

/** Query keys are grouped by domain so a websocket notification can invalidate
 * a whole branch (`keys.contacts.all`) without knowing every leaf. */
export const keys = {
  sessions: {
    all: ["sessions"] as const,
    user: (userId: string) => ["sessions", "user", userId] as const,
    group: (userId: string) => ["sessions", "group", userId] as const,
    open: (userId: string, contactId: string) =>
      ["sessions", "open", userId, contactId] as const,
  },
  contacts: {
    all: ["contacts"] as const,
    friends: (userId: string) => ["contacts", "friends", userId] as const,
    tags: (userId: string) => ["contacts", "tags", userId] as const,
    info: (userId: string, contactId: string) =>
      ["contacts", "info", userId, contactId] as const,
    contactApplies: (userId: string) =>
      ["contacts", "applies", "contact", userId] as const,
    groupApplies: (userId: string) =>
      ["contacts", "applies", "group", userId] as const,
    userSearch: (userId: string, keyword: string) =>
      ["contacts", "search", "user", userId, keyword] as const,
    groupSearch: (userId: string, keyword: string) =>
      ["contacts", "search", "group", userId, keyword] as const,
  },
  groups: {
    all: ["groups"] as const,
    mine: (userId: string) => ["groups", "mine", userId] as const,
    joined: (userId: string) => ["groups", "joined", userId] as const,
    info: (groupId: string) => ["groups", "info", groupId] as const,
    members: (groupId: string) => ["groups", "members", groupId] as const,
    adminList: ["groups", "admin"] as const,
  },
  messages: {
    all: ["messages"] as const,
    with: (userId: string, contactId: string) =>
      ["messages", userId, contactId] as const,
  },
  users: {
    all: ["users"] as const,
    profile: (userId: string) => ["users", "profile", userId] as const,
    adminList: (userId: string) => ["users", "admin", userId] as const,
  },
  chatroom: {
    all: ["chatroom"] as const,
    online: ["chatroom", "online"] as const,
    callers: (roomId: string, userId: string) =>
      ["chatroom", "callers", roomId, userId] as const,
  },
};

/** Module scope keeps the reference stable so react-query can memoize it. */
const newestFirst = (rows: Array<UserSession | GroupSession>): ChatSession[] =>
  orderBy(rows.map(toChatSession), ["lastMessageAtMs"], ["desc"]);

export const userSessionsQuery = (userId: string) =>
  queryOptions({
    queryKey: keys.sessions.user(userId),
    queryFn: ({ signal }) => session.userList(userId, signal),
    select: newestFirst,
    enabled: Boolean(userId),
  });

export const groupSessionsQuery = (userId: string) =>
  queryOptions({
    queryKey: keys.sessions.group(userId),
    queryFn: ({ signal }) => session.groupList(userId, signal),
    select: newestFirst,
    enabled: Boolean(userId),
  });

/** Opening a session is idempotent, so it reads as a query even though it POSTs. */
export const openSessionQuery = (userId: string, contactId: string) =>
  queryOptions({
    queryKey: keys.sessions.open(userId, contactId),
    queryFn: ({ signal }) => session.open(userId, contactId, signal),
    enabled: Boolean(userId && contactId),
    staleTime: "static",
  });

export const contactInfoQuery = (userId: string, contactId: string) =>
  queryOptions({
    queryKey: keys.contacts.info(userId, contactId),
    queryFn: ({ signal }) => contact.info(userId, contactId, signal),
    enabled: Boolean(userId && contactId),
  });

export const friendsQuery = (userId: string) =>
  queryOptions({
    queryKey: keys.contacts.friends(userId),
    queryFn: ({ signal }) => contact.friends(userId, signal),
    enabled: Boolean(userId),
  });

export const tagsQuery = (userId: string) =>
  queryOptions({
    queryKey: keys.contacts.tags(userId),
    queryFn: ({ signal }) => contact.tags(userId, signal),
    enabled: Boolean(userId),
  });

export const contactAppliesQuery = (userId: string) =>
  queryOptions({
    queryKey: keys.contacts.contactApplies(userId),
    queryFn: ({ signal }) => contact.contactApplies(userId, signal),
    enabled: Boolean(userId),
  });

export const groupAppliesQuery = (userId: string) =>
  queryOptions({
    queryKey: keys.contacts.groupApplies(userId),
    queryFn: ({ signal }) => contact.groupApplies(userId, signal),
    enabled: Boolean(userId),
  });

export const userSearchQuery = (userId: string, keyword: string) =>
  queryOptions({
    queryKey: keys.contacts.userSearch(userId, keyword),
    queryFn: ({ signal }) => user.search(userId, keyword, signal),
    enabled: Boolean(userId && keyword),
  });

export const groupSearchQuery = (userId: string, keyword: string) =>
  queryOptions({
    queryKey: keys.contacts.groupSearch(userId, keyword),
    queryFn: ({ signal }) => group.search(userId, keyword, signal),
    enabled: Boolean(userId && keyword),
  });

export const myGroupsQuery = (userId: string) =>
  queryOptions({
    queryKey: keys.groups.mine(userId),
    queryFn: ({ signal }) => group.mine(userId, signal),
    enabled: Boolean(userId),
  });

export const joinedGroupsQuery = (userId: string) =>
  queryOptions({
    queryKey: keys.groups.joined(userId),
    queryFn: ({ signal }) => contact.joinedGroups(userId, signal),
    enabled: Boolean(userId),
  });

export const groupInfoQuery = (groupId: string) =>
  queryOptions({
    queryKey: keys.groups.info(groupId),
    queryFn: ({ signal }) => group.info(groupId, signal),
    enabled: Boolean(groupId),
  });

export const groupMembersQuery = (groupId: string) =>
  queryOptions({
    queryKey: keys.groups.members(groupId),
    queryFn: ({ signal }) => group.members(groupId, signal),
    enabled: Boolean(groupId),
  });

export const messagesQuery = (userId: string, contactId: string) =>
  queryOptions({
    queryKey: keys.messages.with(userId, contactId),
    queryFn: ({ signal }) =>
      isGroupId(contactId)
        ? message.withGroup(contactId, signal)
        : message.withUser(userId, contactId, signal),
    enabled: Boolean(userId && contactId),
  });

export const profileQuery = (userId: string) =>
  queryOptions({
    queryKey: keys.users.profile(userId),
    queryFn: ({ signal }) => user.get(userId, signal),
    enabled: Boolean(userId),
  });

export const adminUsersQuery = (userId: string) =>
  queryOptions({
    queryKey: keys.users.adminList(userId),
    queryFn: ({ signal }) => user.listAll(userId, signal),
    enabled: Boolean(userId),
  });

export const adminGroupsQuery = () =>
  queryOptions({
    queryKey: keys.groups.adminList,
    queryFn: ({ signal }) => group.listAll(signal),
  });

export const onlineUsersQuery = () =>
  queryOptions({
    queryKey: keys.chatroom.online,
    queryFn: ({ signal }) => chatroom.onlineUsers(signal),
  });

export const callersQuery = (roomId: string, userId: string) =>
  queryOptions({
    queryKey: keys.chatroom.callers(roomId, userId),
    queryFn: ({ signal }) => chatroom.callers(roomId, signal),
    enabled: Boolean(roomId && userId),
  });
