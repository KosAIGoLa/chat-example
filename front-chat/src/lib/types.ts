export interface UserInfo {
	id: number;
	username: string;
	balance?: number;
	/** Public path e.g. /api/avatar/11 */
	avatar?: string;
	/** Cache-bust revision */
	avatar_rev?: number;
}

export interface LoginResponse {
	token: string;
	user: UserInfo;
}

export interface APIResponse<T = unknown> {
	code: number;
	message: string;
	data?: T;
}

/** @deprecated import from `$lib/chat` instead */
export type { ChatMessage } from './chat/types';

export interface OnlineUser {
	user_id: string;
	username: string;
}

export interface OnlineUsersResponse {
	online_users: OnlineUser[];
	count: number;
}
