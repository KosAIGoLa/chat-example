import type {
	CryptoKeyResponse,
	FriendRequest,
	FriendUser,
	GroupInfo,
	GroupMembersResponse,
	HistoryResponse,
	VoiceUploadResult
} from './chat/types';
import type { APIResponse, LoginResponse, OnlineUsersResponse, UserInfo } from './types';

const API_BASE = import.meta.env.VITE_API_BASE ?? '';

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
	const token = localStorage.getItem('token');
	const headers: Record<string, string> = {
		'Content-Type': 'application/json',
		...((options.headers as Record<string, string>) ?? {})
	};
	if (token) {
		headers['Authorization'] = `Bearer ${token}`;
	}

	const res = await fetch(`${API_BASE}${path}`, { ...options, headers });
	const body: APIResponse<T> = await res.json();

	if (body.code >= 400) {
		throw new Error(body.message);
	}
	return body.data as T;
}

export const api = {
	register: async (username: string, password: string): Promise<UserInfo> => {
		return request<UserInfo>('/api/auth/register', {
			method: 'POST',
			body: JSON.stringify({ username, password })
		});
	},

	login: async (username: string, password: string): Promise<LoginResponse> => {
		return request<LoginResponse>('/api/auth/login', {
			method: 'POST',
			body: JSON.stringify({ username, password })
		});
	},

	getMe: async (): Promise<UserInfo> => {
		return request<UserInfo>('/api/auth/me');
	},

	/** AES-GCM key for encrypting WebSocket message content. */
	getCryptoKey: async (): Promise<CryptoKeyResponse> => {
		return request<CryptoKeyResponse>('/api/crypto/key');
	},

	updateProfile: async (body: {
		username: string;
		password?: string;
		current_password?: string;
	}): Promise<LoginResponse> => {
		return request<LoginResponse>('/api/auth/profile', {
			method: 'PUT',
			body: JSON.stringify(body)
		});
	},

	getOnlineUsers: async (): Promise<OnlineUsersResponse> => {
		return request<OnlineUsersResponse>('/api/users/online');
	},

	/** Accepted friends (with online flag). */
	listFriends: async (): Promise<{ friends: FriendUser[]; count: number }> => {
		return request('/api/friends');
	},

	listIncomingFriendRequests: async (): Promise<{ requests: FriendRequest[]; count: number }> => {
		return request('/api/friends/requests/incoming');
	},

	listOutgoingFriendRequests: async (): Promise<{ requests: FriendRequest[]; count: number }> => {
		return request('/api/friends/requests/outgoing');
	},

	/** Invite by username (preferred) or user_id. Pending until the other accepts. */
	inviteFriend: async (opts: { username?: string; user_id?: string }): Promise<FriendRequest> => {
		return request('/api/friends/request', {
			method: 'POST',
			body: JSON.stringify(opts)
		});
	},

	acceptFriendRequest: async (id: number): Promise<FriendRequest> => {
		return request(`/api/friends/requests/${id}/accept`, { method: 'POST' });
	},

	rejectFriendRequest: async (id: number): Promise<FriendRequest> => {
		return request(`/api/friends/requests/${id}/reject`, { method: 'POST' });
	},

	removeFriend: async (userId: string): Promise<void> => {
		await request(`/api/friends/${encodeURIComponent(userId)}`, { method: 'DELETE' });
	},

	/** Create a durable group (caller becomes owner). */
	createGroup: async (opts?: { name?: string; group_id?: string }): Promise<GroupInfo> => {
		return request<GroupInfo>('/api/groups', {
			method: 'POST',
			body: JSON.stringify(opts ?? {})
		});
	},

	/** List groups I belong to. */
	listMyGroups: async (): Promise<{ groups: GroupInfo[]; count: number }> => {
		return request('/api/groups');
	},

	/** Owner-only: dissolve group and kick all members. */
	dissolveGroup: async (groupId: string): Promise<{ group_id: string; name: string }> => {
		return request(`/api/groups/${encodeURIComponent(groupId)}/dissolve`, { method: 'POST' });
	},

	/**
	 * Join a group while online.
	 * rejoin=true: restore membership after reconnect — no "加入到群" broadcast.
	 */
	joinGroup: async (groupId: string, opts?: { rejoin?: boolean }): Promise<unknown> => {
		const q = new URLSearchParams({ group_id: groupId });
		if (opts?.rejoin) q.set('rejoin', '1');
		return request(`/api/groups/join?${q}`, { method: 'POST' });
	},

	leaveGroup: async (groupId: string, opts?: { silent?: boolean }): Promise<unknown> => {
		const q = new URLSearchParams({ group_id: groupId });
		if (opts?.silent) q.set('silent', '1');
		return request(`/api/groups/leave?${q}`, { method: 'POST' });
	},

	getGroupMembers: async (groupId: string): Promise<GroupMembersResponse> => {
		return request<GroupMembersResponse>(`/api/groups/${encodeURIComponent(groupId)}/members`);
	},

	getPrivateHistory: async (peerId: string, count = 50): Promise<HistoryResponse> => {
		const q = new URLSearchParams({
			type: 'private',
			peer_id: peerId,
			count: String(count)
		});
		return request<HistoryResponse>(`/api/history?${q}`);
	},

	getGroupHistory: async (groupId: string, count = 50): Promise<HistoryResponse> => {
		const q = new URLSearchParams({
			type: 'group',
			group_id: groupId,
			count: String(count)
		});
		return request<HistoryResponse>(`/api/history?${q}`);
	},

	/** Upload a recorded voice blob. */
	uploadVoice: async (blob: Blob, durationSec: number): Promise<VoiceUploadResult> => {
		const token = localStorage.getItem('token');
		if (!token) {
			throw new Error('Not logged in');
		}
		// Normalize browser quirks: Chrome often labels audio-only as video/webm.
		const fixed = normalizeVoiceBlob(blob);
		const form = new FormData();
		const ext = guessAudioExt(fixed.type);
		form.append('file', fixed, `voice${ext}`);
		form.append('duration', String(Math.max(0, durationSec)));

		const res = await fetch(`${API_BASE}/api/voice`, {
			method: 'POST',
			headers: { Authorization: `Bearer ${token}` },
			body: form
		});

		let body: APIResponse<VoiceUploadResult>;
		try {
			body = await res.json();
		} catch {
			throw new Error(
				res.ok ? 'Invalid upload response' : `Upload failed (HTTP ${res.status})`
			);
		}
		if (!res.ok || body.code >= 400) {
			throw new Error(body.message || `Upload failed (HTTP ${res.status})`);
		}
		if (!body.data?.url) {
			throw new Error('Upload succeeded but no media URL returned');
		}
		return body.data;
	}
};

/** Fix MediaRecorder MIME labels so the server accepts the file. */
function normalizeVoiceBlob(blob: Blob): Blob {
	const raw = (blob.type || '').split(';')[0].trim().toLowerCase();
	let type = raw;
	if (!type || type === 'application/octet-stream') {
		type = 'audio/webm';
	} else if (type === 'video/webm') {
		// Audio-only WebM from MediaRecorder.
		type = 'audio/webm';
	} else if (type === 'video/mp4') {
		type = 'audio/mp4';
	}
	if (type === blob.type) return blob;
	return new Blob([blob], { type });
}

function guessAudioExt(mime: string): string {
	const m = (mime || '').split(';')[0].trim().toLowerCase();
	if (m.includes('webm')) return '.webm';
	if (m.includes('ogg')) return '.ogg';
	if (m.includes('mp4') || m.includes('m4a') || m.includes('aac')) return '.m4a';
	if (m.includes('mpeg') || m.includes('mp3')) return '.mp3';
	if (m.includes('wav')) return '.wav';
	return '.webm';
}

export function buildWsUrl(token: string): string {
	const base = import.meta.env.VITE_WS_BASE ?? window.location.origin;
	const wsBase = base.replace(/^http/, 'ws');
	return `${wsBase}/ws?token=${encodeURIComponent(token)}`;
}

/** Build an authenticated media URL suitable for <audio src>. */
export function buildMediaUrl(path: string): string {
	if (!path) return '';
	if (path.startsWith('http://') || path.startsWith('https://') || path.startsWith('blob:')) {
		return path;
	}
	const token = localStorage.getItem('token') ?? '';
	const base = API_BASE || '';
	const sep = path.includes('?') ? '&' : '?';
	return `${base}${path}${sep}token=${encodeURIComponent(token)}`;
}
