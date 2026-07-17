import type { CryptoKeyResponse, HistoryResponse } from '$lib/chat/types';
import type { OnlineUsersResponse } from '$lib/types';
import { request } from './client';

export type HistoryOpts = {
	/** Incremental: only messages with seq > sinceSeq */
	sinceSeq?: number;
	sinceTs?: number;
	/** Scroll-up: only messages older than beforeSeq / beforeTs */
	beforeSeq?: number;
	beforeTs?: number;
};

/** Chat transport helpers: crypto key, presence list, message history. */
export const chatService = {
	/** AES-GCM key fopeerId: string, count = 50, p0: { beforeSeq: number | undefined; beforeTs: number | undefined; }t encryption. */
	getCryptoKey(): Promise<CryptoKeyResponse> {
		return request<CryptoKeyResponse>('/api/crypto/key');
	},

	getOnlineUsers(): Promise<OnlineUsersResponse> {
		return request<OnlineUsersResponse>('/api/users/online');
	},

	/**
	 * Private history (server retains ~6 months).
	 * - since_seq: newer than cache (delta sync)
	 * - before_seq: older page when scrolling up
	 */
	getPrivateHistory(
		peerId: string,
		count = 80,
		opts?: HistoryOpts
	): Promise<HistoryResponse> {
		const q = new URLSearchParams({
			type: 'private',
			peer_id: peerId,
			count: String(count)
		});
		if (opts?.beforeSeq && opts.beforeSeq > 0) {
			q.set('before_seq', String(opts.beforeSeq));
			if (opts.beforeTs && opts.beforeTs > 0) q.set('before_ts', String(opts.beforeTs));
		} else if (opts?.sinceSeq && opts.sinceSeq > 0) {
			q.set('since_seq', String(opts.sinceSeq));
		} else if (opts?.sinceTs && opts.sinceTs > 0) {
			q.set('since_ts', String(opts.sinceTs));
		} else if (opts?.beforeTs && opts.beforeTs > 0) {
			q.set('before_ts', String(opts.beforeTs));
		}
		return request<HistoryResponse>(`/api/history?${q}`);
	},

	getGroupHistory(groupId: string, count = 80, opts?: HistoryOpts): Promise<HistoryResponse> {
		const q = new URLSearchParams({
			type: 'group',
			group_id: groupId,
			count: String(count)
		});
		if (opts?.beforeSeq && opts.beforeSeq > 0) {
			q.set('before_seq', String(opts.beforeSeq));
			if (opts.beforeTs && opts.beforeTs > 0) q.set('before_ts', String(opts.beforeTs));
		} else if (opts?.sinceSeq && opts.sinceSeq > 0) {
			q.set('since_seq', String(opts.sinceSeq));
		} else if (opts?.sinceTs && opts.sinceTs > 0) {
			q.set('since_ts', String(opts.sinceTs));
		} else if (opts?.beforeTs && opts.beforeTs > 0) {
			q.set('before_ts', String(opts.beforeTs));
		}
		return request<HistoryResponse>(`/api/history?${q}`);
	}
};
