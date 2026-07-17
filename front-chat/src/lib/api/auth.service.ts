import type { LoginResponse, UserInfo } from '$lib/types';
import { request, requestForm } from './client';

export interface AvatarUploadResult {
	avatar: string;
	avatar_rev: number;
	url: string;
}

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
	},

	/** Upload profile avatar image (jpeg/png/webp/gif, max 2MB). */
	uploadAvatar(file: File | Blob): Promise<AvatarUploadResult> {
		const form = new FormData();
		form.append('file', file, file instanceof File ? file.name : 'avatar.jpg');
		return requestForm<AvatarUploadResult>('/api/avatar', form);
	}
};

/** Build avatar URL for a user id (with optional cache-bust rev). */
export function avatarUrl(userId: string | number, rev?: number): string {
	if (userId === '' || userId == null) return '';
	const base = `/api/avatar/${userId}`;
	return rev && rev > 0 ? `${base}?v=${rev}` : base;
}
