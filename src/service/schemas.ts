import * as z from "zod";

import { resolveAvatar } from "@/utils/avatar";

/** `Message.type` wire values (swifty_chat/internal/constant/constant.go). */
export const MessageType = {
  Text: 0,
  Image: 1,
  File: 2,
  AvSignal: 3,
  Video: 4,
  System: 5,
} as const;

/** A system frame's `content` names the list that went stale. */
export const SystemTopic = {
  Contact: "contact",
  Group: "group",
  Apply: "apply",
  Session: "session",
  Online: "online",
} as const;

export type SystemTopicValue = (typeof SystemTopic)[keyof typeof SystemTopic];

export const SYSTEM_SENDER = "SYSTEM";

/** The backend prefixes user uuids with "U" and group uuids with "G". */
export const isUserId = (id: string) => id.startsWith("U");
export const isGroupId = (id: string) => id.startsWith("G");

/** Go marshals empty slices as `null`. */
export const wireList = <T extends z.ZodType>(item: T) =>
  z
    .array(item)
    .nullish()
    .transform((value) => value ?? []);

const list = wireList;

const text = z.string().default("");
const count = z.number().default(0);

export const userInfoSchema = z
  .object({
    uuid: z.string(),
    telephone: text,
    nickname: text,
    email: text,
    avatar: text,
    gender: count,
    birthday: text,
    signature: text,
    is_admin: count,
    status: count,
    created_at: text,
  })
  .transform((user) => ({
    ...user,
    avatar: resolveAvatar(user.avatar, user.uuid),
  }));
export type UserInfo = z.infer<typeof userInfoSchema>;

export const authSchema = z.object({
  token: z.string(),
  user_info: userInfoSchema,
});
export type AuthResult = z.infer<typeof authSchema>;

export const adminUserSchema = z.object({
  uuid: z.string(),
  telephone: text,
  nickname: text,
  status: count,
  is_admin: count,
  is_deleted: z.boolean().default(false),
});
export type AdminUser = z.infer<typeof adminUserSchema>;

export const userSearchResultSchema = z
  .object({
    uuid: z.string(),
    nickname: text,
    telephone: text,
    avatar: text,
    is_friend: z.boolean().default(false),
  })
  .transform((user) => ({
    ...user,
    avatar: resolveAvatar(user.avatar, user.uuid),
  }));
export type UserSearchResult = z.infer<typeof userSearchResultSchema>;

export const contactInfoSchema = z
  .object({
    contact_id: z.string(),
    contact_name: text,
    contact_avatar: text,
    contact_phone: text,
    contact_email: text,
    contact_gender: count,
    contact_signature: text,
    contact_birthday: text,
    contact_notice: text,
    contact_members: list(z.string()),
    contact_member_cnt: count,
    contact_owner_id: text,
    contact_add_mode: count,
  })
  .transform((contact) => ({
    ...contact,
    contact_avatar: resolveAvatar(contact.contact_avatar, contact.contact_id),
  }));
export type ContactInfo = z.infer<typeof contactInfoSchema>;

export const applySchema = z.object({
  apply_id: z.string(),
  user_id: text,
  contact_id: text,
  contact_name: text,
  contact_type: count,
  status: count,
  message: text,
});
export type Apply = z.infer<typeof applySchema>;

export const friendSchema = z
  .object({
    user_id: z.string(),
    nickname: text,
    avatar: text,
    status: count,
    note_name: text,
    tag_id: text,
    online: z.boolean().default(false),
  })
  .transform((friend) => ({
    ...friend,
    avatar: resolveAvatar(friend.avatar, friend.user_id),
  }));
export type Friend = z.infer<typeof friendSchema>;

export const tagSchema = z.object({ tag_id: z.string(), name: text });
export type Tag = z.infer<typeof tagSchema>;

const sessionFields = {
  session_id: text,
  avatar: text,
  last_message: text,
  last_message_type: count,
  last_message_at: text,
  last_message_at_ms: count,
  unread_cnt: count,
};

export const userSessionSchema = z.object({
  ...sessionFields,
  user_id: z.string(),
  username: text,
});
export type UserSession = z.infer<typeof userSessionSchema>;

export const groupSessionSchema = z.object({
  ...sessionFields,
  group_id: z.string(),
  group_name: text,
});
export type GroupSession = z.infer<typeof groupSessionSchema>;

/** `get-user-session-list` and `get-group-session-list` differ only in the
 * id/name field names; the sidebar renders both from one shape. */
export interface ChatSession {
  kind: "user" | "group";
  id: string;
  name: string;
  avatar: string;
  sessionId: string;
  lastMessage: string;
  lastMessageType: number;
  lastMessageAtMs: number;
  unreadCount: number;
}

export function toChatSession(
  raw: z.infer<typeof userSessionSchema> | z.infer<typeof groupSessionSchema>,
): ChatSession {
  const isGroup = "group_id" in raw;
  const id = isGroup ? raw.group_id : raw.user_id;
  return {
    kind: isGroup ? "group" : "user",
    id,
    name: (isGroup ? raw.group_name : raw.username) || id,
    avatar: resolveAvatar(raw.avatar, id),
    sessionId: raw.session_id,
    lastMessage: raw.last_message,
    lastMessageType: raw.last_message_type,
    lastMessageAtMs: raw.last_message_at_ms,
    unreadCount: raw.unread_cnt,
  };
}

export const messageSchema = z
  .object({
    uuid: text,
    send_id: z.string(),
    send_name: text,
    send_avatar: text,
    receive_id: text,
    type: count,
    content: text,
    url: text,
    file_size: text,
    file_name: text,
    file_type: text,
    created_at: text,
    av_data: z.string().optional(),
  })
  .transform((message) => ({
    ...message,
    send_avatar: resolveAvatar(message.send_avatar, message.send_id),
  }));
export type Message = z.infer<typeof messageSchema>;

/** The envelope the client writes to the socket (chat_server.go ChatMessageRequest). */
export interface OutgoingFrame {
  session_id: string;
  type: number;
  content: string;
  url: string;
  send_id: string;
  send_name: string;
  send_avatar: string;
  receive_id: string;
  file_size: string;
  file_name: string;
  file_type: string;
  av_data?: string;
}

export const groupInfoSchema = z
  .object({
    uuid: z.string(),
    name: text,
    notice: text,
    members: list(z.string()),
    member_cnt: count,
    owner_id: text,
    add_mode: count,
    avatar: text,
    status: count,
  })
  .transform((group) => ({
    ...group,
    avatar: resolveAvatar(group.avatar, group.uuid),
  }));
export type GroupInfo = z.infer<typeof groupInfoSchema>;

export const myGroupSchema = z
  .object({
    group_id: z.string(),
    name: text,
    member_cnt: count,
    owner_id: text,
    avatar: text,
  })
  .transform((group) => ({
    ...group,
    avatar: resolveAvatar(group.avatar, group.group_id),
  }));
export type MyGroup = z.infer<typeof myGroupSchema>;

export const adminGroupSchema = z.object({
  group_id: z.string(),
  name: text,
  member_cnt: count,
  owner_id: text,
  avatar: text,
  status: count,
  is_deleted: z.boolean().default(false),
});
export type AdminGroup = z.infer<typeof adminGroupSchema>;

export const groupSearchResultSchema = z
  .object({
    group_id: z.string(),
    name: text,
    avatar: text,
    member_cnt: count,
    add_mode: count,
    is_joined: z.boolean().default(false),
  })
  .transform((group) => ({
    ...group,
    avatar: resolveAvatar(group.avatar, group.group_id),
  }));
export type GroupSearchResult = z.infer<typeof groupSearchResultSchema>;

export const groupMemberSchema = z
  .object({
    user_id: z.string(),
    uuid: text,
    nickname: text,
    avatar: text,
    is_owner: z.boolean().default(false),
    joined_at: text,
    last_message_at: text,
  })
  .transform((member) => ({
    ...member,
    avatar: resolveAvatar(member.avatar, member.user_id),
  }));
export type GroupMember = z.infer<typeof groupMemberSchema>;

export const chunkVerifySchema = z.union([
  z.object({ uploaded: z.literal(true), url: z.string() }),
  z.object({
    uploaded: z.literal(false),
    pending_chunks: list(z.number()),
  }),
]);
export type ChunkVerifyResult = z.infer<typeof chunkVerifySchema>;

export const uploadedFileSchema = z.object({
  url: z.string(),
  file_name: text,
  file_size: text,
});
export type UploadedFile = z.infer<typeof uploadedFileSchema>;

export const uploadedAvatarSchema = z.object({ url: z.string() });
