import type { LoginResponse, UserInfo } from '$lib/types';
import { request } from './client';

/** Auth & profile REST API. */
export const authService = {
	register(username: string, password: string): Promise<UserInfo> {
		return request<UserInfo>('/api/auth/register', {
			method: 'POST',
			body: JSON.stringify({ username, password })
		});
	},

	login(username: string, password: string): Promise<LoginResponse> {
		return request<LoginResponse>('/api/auth/login', {
			method: 'POST',
			body: JSON.stringify({ username, password })
		});
	},

	getMe(): Promise<UserInfo> {
		return request<UserInfo>('/api/auth/me');
	},

	updateProfile(body: {
		username: string;
		password?: string;
		current_password?: string;
	}): Promise<LoginResponse> {
		return request<LoginResponse>('/api/auth/profile', {
			method: 'PUT',
			body: JSON.stringify(body)
		});
	}
};
