export interface UserInfo {
	id: number;
	username: string;
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
