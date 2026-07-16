export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';
export type ChatMode = 'private' | 'group';
export type ContentType = 'text' | 'voice';

export interface ChatMessage {
	type: 'private' | 'group' | 'join_group' | 'leave_group' | 'history' | 'presence';
	from: string;
	to: string;
	/** Plaintext locally after decrypt; on wire may be enc:v1:… ciphertext. */
	content: string;
	group_id?: string;
	timestamp?: number;
	/** "text" (default) or "voice" */
	content_type?: ContentType | string;
	/** Voice file URL path, e.g. /api/voice/xxx.webm */
	media_url?: string;
	/** Voice duration in seconds */
	duration?: number;
	/** True when content is AES-GCM ciphertext. */
	encrypted?: boolean;
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

export interface ChatUser {
	id: number;
	username: string;
}

export interface HistoryResponse {
	messages: ChatMessage[];
	count: number;
}
