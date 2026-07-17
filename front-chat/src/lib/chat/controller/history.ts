/**
 * Conversation history: cache-first paint + server pages (since_seq / before_seq).
 */

import { chatService } from '$lib/api';
import type { ChatMessage, ChatMode } from '../types';
import {
	convKeyGroup,
	convKeyPrivate,
	loadConvCache,
	maxSeqOf,
	mergeById,
	minSeqOf,
	minTimestampOf,
	saveConvCache,
	sortMessagesBySeq,
	clearConvCache,
	clearAllConvCaches
} from '../message-cache';
import {
	decryptMessages,
	filterBlockedMessages,
	isChatContent
} from './message-helpers';
import { HISTORY_PAGE } from './constants';

export interface HistoryDeps {
	getMyUserId: () => string;
	getChatMode: () => ChatMode;
	getTargetUser: () => string;
	getGroupId: () => string;
	getMessages: () => ChatMessage[];
	setMessages: (m: ChatMessage[]) => void;
	getBlockedIds: () => string[];
	getHistoryEpoch: () => number;
	bumpHistoryEpoch: () => number;
	getLoadedKey: () => string;
	setLoadedKey: (k: string) => void;
	setHistoryLoading: (v: boolean) => void;
	setHistoryLoadingOlder: (v: boolean) => void;
	getHistoryHasMore: () => boolean;
	setHistoryHasMore: (v: boolean) => void;
	ensureCryptoKey: () => Promise<void>;
	updatePreview: (msg: ChatMessage) => void;
	hasMessageKey: () => boolean;
}

export function createHistoryApi(deps: HistoryDeps) {
	async function loadConversationHistory(opts: {
		key: string;
		force: boolean;
		fetch: (
			page: number,
			sinceSeq: number
		) => Promise<{ messages?: ChatMessage[]; max_seq?: number; has_more?: boolean }>;
	}) {
		const { key, force, fetch } = opts;
		if (!key || key.endsWith(':')) return;
		if (!force && deps.getLoadedKey() === key) return;

		const epoch = deps.bumpHistoryEpoch();
		const switching = deps.getLoadedKey() !== key;
		deps.setLoadedKey(key);
		if (switching) deps.setHistoryHasMore(true);

		const cached = loadConvCache(key);
		let base: ChatMessage[];
		let paintedFromCache = false;
		const blocked = deps.getBlockedIds();
		const myId = deps.getMyUserId();

		if (cached?.messages?.length) {
			const cachedMsgs = sortMessagesBySeq(
				filterBlockedMessages([...cached.messages], myId, blocked)
			);
			if (switching || deps.getMessages().length === 0) {
				base = cachedMsgs;
				deps.setMessages(base);
				const tail = base.slice(-30);
				for (const m of tail) deps.updatePreview(m);
				paintedFromCache = true;
			} else {
				base = mergeById(deps.getMessages(), cachedMsgs);
				deps.setMessages(base);
				paintedFromCache = true;
			}
		} else if (switching) {
			deps.setMessages([]);
			base = [];
		} else {
			base = filterBlockedMessages([...deps.getMessages()], myId, blocked);
		}

		deps.setHistoryLoading(!paintedFromCache);

		try {
			if (!deps.hasMessageKey()) {
				await deps.ensureCryptoKey().catch(() => undefined);
			}
			const sinceSeq = maxSeqOf(base);
			const res = await fetch(HISTORY_PAGE, sinceSeq);
			if (epoch !== deps.getHistoryEpoch()) return;

			const rawList = (res.messages ?? []).filter(isChatContent);
			const list = filterBlockedMessages(await decryptMessages(rawList), myId, blocked);
			if (epoch !== deps.getHistoryEpoch()) return;

			if (list.length > 0) {
				const merged = filterBlockedMessages(mergeById(base, list), myId, blocked);
				deps.setMessages(merged);
				for (const m of list) deps.updatePreview(m);
				const maxSeq = Math.max(sinceSeq, res.max_seq ?? 0, maxSeqOf(merged));
				queueMicrotask(() => saveConvCache(key, merged, maxSeq));
			} else if (sinceSeq === 0) {
				deps.setMessages(base);
				queueMicrotask(() => saveConvCache(key, base, 0));
			} else {
				queueMicrotask(() =>
					saveConvCache(key, base, Math.max(sinceSeq, res.max_seq ?? 0))
				);
			}

			if (sinceSeq === 0) {
				if (typeof res.has_more === 'boolean') {
					deps.setHistoryHasMore(res.has_more);
				} else {
					deps.setHistoryHasMore(list.length >= HISTORY_PAGE);
				}
			}
		} catch (e) {
			console.warn('[history] load failed', key, e);
			if (epoch === deps.getHistoryEpoch() && !base.length) deps.setMessages([]);
		} finally {
			if (epoch === deps.getHistoryEpoch()) deps.setHistoryLoading(false);
		}
	}

	async function loadPrivateHistory(peerId: string, force = false) {
		const peer = (peerId || '').trim();
		if (!peer) return;
		await loadConversationHistory({
			key: convKeyPrivate(peer),
			force,
			fetch: (page, sinceSeq) =>
				chatService.getPrivateHistory(peer, page, {
					sinceSeq: sinceSeq > 0 ? sinceSeq : undefined
				})
		});
	}

	async function loadGroupHistory(g: string, force = false) {
		const gid = (g || '').trim();
		if (!gid) return;
		await loadConversationHistory({
			key: convKeyGroup(gid),
			force,
			fetch: (page, sinceSeq) =>
				chatService.getGroupHistory(gid, page, {
					sinceSeq: sinceSeq > 0 ? sinceSeq : undefined
				})
		});
	}

	async function loadOlderHistory(): Promise<number> {
		const mode = deps.getChatMode();
		const peer = deps.getTargetUser().trim();
		const gid = deps.getGroupId().trim();
		if (mode === 'private' && !peer) return 0;
		if (mode === 'group' && !gid) return 0;
		if (!deps.getHistoryHasMore() || deps.getMessages().length === 0) return 0;

		const msgs = deps.getMessages();
		const beforeSeq = minSeqOf(msgs);
		const beforeTs = minTimestampOf(msgs);
		if (beforeSeq <= 0 && beforeTs <= 0) {
			deps.setHistoryHasMore(false);
			return 0;
		}

		deps.setHistoryLoadingOlder(true);
		const epoch = deps.getHistoryEpoch();
		try {
			await deps.ensureCryptoKey().catch(() => undefined);
			const res =
				mode === 'private'
					? await chatService.getPrivateHistory(peer, HISTORY_PAGE, {
							beforeSeq: beforeSeq > 0 ? beforeSeq : undefined,
							beforeTs: beforeTs > 0 ? beforeTs : undefined
						})
					: await chatService.getGroupHistory(gid, HISTORY_PAGE, {
							beforeSeq: beforeSeq > 0 ? beforeSeq : undefined,
							beforeTs: beforeTs > 0 ? beforeTs : undefined
						});
			if (epoch !== deps.getHistoryEpoch()) return 0;
			const list = await decryptMessages((res.messages ?? []).filter(isChatContent));
			const filtered = filterBlockedMessages(
				list,
				deps.getMyUserId(),
				deps.getBlockedIds()
			);
			if (!filtered.length) {
				deps.setHistoryHasMore(false);
				return 0;
			}
			const prevLen = deps.getMessages().length;
			const merged = mergeById(deps.getMessages(), filtered);
			deps.setMessages(merged);
			const added = merged.length - prevLen;
			deps.setHistoryHasMore(res.has_more === true);
			if (added === 0) deps.setHistoryHasMore(false);

			const key = mode === 'private' ? convKeyPrivate(peer) : convKeyGroup(gid);
			saveConvCache(key, merged, maxSeqOf(merged));
			return Math.max(0, added);
		} catch (e) {
			console.warn('[history] load older failed', e);
			return 0;
		} finally {
			if (epoch === deps.getHistoryEpoch()) deps.setHistoryLoadingOlder(false);
		}
	}

	function clearLocalHistory(opts?: { all?: boolean }): number {
		if (opts?.all) {
			const n = clearAllConvCaches();
			deps.setMessages([]);
			deps.setHistoryHasMore(true);
			deps.setLoadedKey('');
			return n;
		}
		const mode = deps.getChatMode();
		const peer = deps.getTargetUser().trim();
		const gid = deps.getGroupId().trim();
		let key = '';
		if (mode === 'private' && peer) key = convKeyPrivate(peer);
		else if (mode === 'group' && gid) key = convKeyGroup(gid);
		if (key) clearConvCache(key);
		deps.setMessages([]);
		deps.setHistoryHasMore(true);
		deps.setLoadedKey('');
		return key ? 1 : 0;
	}

	async function reloadActiveHistory() {
		if (deps.getChatMode() === 'private' && deps.getTargetUser().trim()) {
			await loadPrivateHistory(deps.getTargetUser().trim(), true);
		} else if (deps.getChatMode() === 'group' && deps.getGroupId().trim()) {
			await loadGroupHistory(deps.getGroupId().trim(), true);
		}
	}

	return {
		loadPrivateHistory,
		loadGroupHistory,
		loadOlderHistory,
		clearLocalHistory,
		reloadActiveHistory,
		HISTORY_PAGE
	};
}

export type HistoryApi = ReturnType<typeof createHistoryApi>;
