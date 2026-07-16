import type {
	CryptoKeyResponse,
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

	joinGroup: async (groupId: string): Promise<unknown> => {
		return request(`/api/groups/join?group_id=${encodeURIComponent(groupId)}`, { method: 'POST' });
	},

	leaveGroup: async (groupId: string): Promise<unknown> => {
		return request(`/api/groups/leave?group_id=${encodeURIComponent(groupId)}`, { method: 'POST' });
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
