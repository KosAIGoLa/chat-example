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

	/**
	 * Private history. Prefer since_seq for incremental fetch after last cached message.
	 * since_ts kept for backward compatibility.
	 */
	getPrivateHistory(
		peerId: string,
		count = 50,
		opts?: { sinceSeq?: number; sinceTs?: number }
	): Promise<HistoryResponse> {
		const q = new URLSearchParams({
			type: 'private',
			peer_id: peerId,
			count: String(count)
		});
		if (opts?.sinceSeq && opts.sinceSeq > 0) q.set('since_seq', String(opts.sinceSeq));
		else if (opts?.sinceTs && opts.sinceTs > 0) q.set('since_ts', String(opts.sinceTs));
		return request<HistoryResponse>(`/api/history?${q}`);
	},

	getGroupHistory(
		groupId: string,
		count = 50,
		opts?: { sinceSeq?: number; sinceTs?: number }
	): Promise<HistoryResponse> {
		const q = new URLSearchParams({
			type: 'group',
			group_id: groupId,
			count: String(count)
		});
		if (opts?.sinceSeq && opts.sinceSeq > 0) q.set('since_seq', String(opts.sinceSeq));
		else if (opts?.sinceTs && opts.sinceTs > 0) q.set('since_ts', String(opts.sinceTs));
		return request<HistoryResponse>(`/api/history?${q}`);
	}
};
