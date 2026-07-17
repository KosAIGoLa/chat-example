export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';
export type ChatMode = 'private' | 'group';
export type ContentType = 'text' | 'voice' | 'system';

/** Application-level WS heartbeat (client → server). */
export interface PingMessage {
	type: 'ping';
	/** Client clock ms — echoed back on pong for RTT. */
	ts: number;
}

/** Application-level WS heartbeat reply (server → client). */
export interface PongMessage {
	type: 'pong';
	ts?: number;
	server_ts?: number;
}

export interface ChatMessage {
	/** Stable id for recall (client-generated UUID hex / server-issued). */
	id?: string;
	type:
		| 'private'
		| 'group'
		| 'join_group'
		| 'leave_group'
		| 'history'
		| 'presence'
		| 'ping'
		| 'pong'
		| 'recall'
		| 'error'
		| 'friend_event';
	from: string;
	to: string;
	/** Plaintext locally after decrypt; on wire may be enc:v1:… ciphertext. */
	content: string;
	group_id?: string;
	timestamp?: number;
	/** "text" (default) or "voice" | "system" */
	content_type?: ContentType | string;
	/** Voice file URL path, e.g. /api/voice/xxx.webm */
	media_url?: string;
	/** Voice duration in seconds */
	duration?: number;
	/** True when content is AES-GCM ciphertext. */
	encrypted?: boolean;
	/** True after successful recall within the window. */
	recalled?: boolean;
}

/** Server push when a message is recalled. */
export interface RecallEvent {
	type: 'recall';
	id: string;
	from: string;
	to?: string;
	group_id?: string;
	timestamp?: number;
}

/** How long (ms) the sender may recall a message. Must match server RecallWindow. */
export const RECALL_WINDOW_MS = 2 * 60 * 1000;

export interface FriendUser {
	user_id: string;
	username: string;
	online: boolean;
}

export interface FriendRequest {
	id: number;
	from_user_id: string;
	from_username: string;
	to_user_id: string;
	to_username: string;
	status: 'pending' | 'accepted' | 'rejected' | string;
	created_at: number;
}

export interface FriendEvent {
	type: 'friend_event';
	action: 'request' | 'accepted' | 'rejected' | 'removed' | 'blocked' | string;
	request_id?: number;
	from_user_id?: string;
	from_username?: string;
	to_user_id?: string;
	to_username?: string;
}

/** One entry in my blacklist. */
export interface BlacklistUser {
	user_id: string;
	username: string;
	created_at: number;
}

/** audio = 语音通话, video = 视讯通话 */
export type CallMedia = 'audio' | 'video';

/** WebRTC call signaling over chat WebSocket (media is LiveKit). */
export interface CallEvent {
	type: 'call';
	action: 'invite' | 'accept' | 'reject' | 'end' | 'cancel' | string;
	room: string;
	call_type: 'private' | 'group' | string;
	/** audio = voice only; video = camera + mic */
	media?: CallMedia | string;
	from: string;
	from_name?: string;
	to?: string;
	group_id?: string;
	timestamp?: number;
}

export interface CryptoKeyResponse {
	algorithm: string;
	key: string;
	version: number;
}

export interface VoiceUploadResult {
	id: string;
	url: string;
	mime_type: string;
	size: number;
	duration: number;
}

/** Real-time presence event pushed over WebSocket. */
export interface PresenceEvent {
	type: 'presence';
	user_id: string;
	username?: string;
	online: boolean;
	instance?: string;
	last_seen?: number;
}

/** Online list entry from REST / presence. */
export interface OnlineUser {
	user_id: string;
	username: string;
}

/** Group roster pushed over WebSocket or loaded via REST. */
export interface GroupMembersEvent {
	type: 'group_members';
	group_id: string;
	members: OnlineUser[];
}

export interface GroupMembersResponse {
	group_id: string;
	members: OnlineUser[];
	count: number;
}

/** Durable group from GET /api/groups. */
export interface GroupInfo {
	id: string;
	name: string;
	owner_user_id: string;
	owner_username?: string;
	role?: 'owner' | 'member' | string;
	member_count?: number;
	created_at?: number;
}

/** Server push when owner dissolves a group. */
export interface GroupDissolvedEvent {
	type: 'group_dissolved';
	group_id: string;
	name?: string;
	by_user_id?: string;
}

export interface ChatUser {
	id: number;
	username: string;
}

export interface HistoryResponse {
	messages: ChatMessage[];
	count: number;
}
