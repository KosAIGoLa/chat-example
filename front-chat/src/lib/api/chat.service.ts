import type { CryptoKeyResponse, HistoryResponse } from '$lib/chat/types';
import type { OnlineUsersResponse } from '$lib/types';
import { request } from './client';

/** Chat transport helpers: crypto key, presence list, message history. */
export const chatService = {
	/** AES-GCM key for WebSocket frame / content encryption. */
	getCryptoKey(): Promise<CryptoKeyResponse> {
		return request<CryptoKeyResponse>('/api/crypto/key');
	},

	getOnlineUsers(): Promise<OnlineUsersResponse> {
		return request<OnlineUsersResponse>('/api/users/online');
	},

	getPrivateHistory(peerId: string, count = 50): Promise<HistoryResponse> {
		const q = new URLSearchParams({
			type: 'private',
			peer_id: peerId,
			count: String(count)
		});
		return request<HistoryResponse>(`/api/history?${q}`);
	},

	getGroupHistory(groupId: string, count = 50): Promise<HistoryResponse> {
		const q = new URLSearchParams({
			type: 'group',
			group_id: groupId,
			count: String(count)
		});
		return request<HistoryResponse>(`/api/history?${q}`);
	}
};
