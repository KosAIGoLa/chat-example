/**
 * Frontend conversation history cache (localStorage).
 * Keyed by conversation: private:<peerId> | group:<groupId>
 * Stores ordered messages + max_seq for incremental since_seq pulls.
 */

import type { ChatMessage } from './types';

const CACHE_PREFIX = 'chat_hist_v1:';
/**
 * Cap localStorage payload size. Too large (thousands) makes every send/open
 * JSON.stringify + localStorage.setItem noticeably slow.
 * Older history is still available via server scroll-up (before_seq).
 */
const MAX_CACHED = 400;

export interface ConvCache {
	/** Conversation key */
	key: string;
	/** Highest known server seq (0 if unknown / legacy only) */
	max_seq: number;
	/** Ordered messages (plaintext preferred after decrypt) */
	messages: ChatMessage[];
	/** Updated at ms */
	updated_at: number;
}

function storageKey(convKey: string): string {
	return CACHE_PREFIX + convKey;
}

export function convKeyPrivate(peerId: string): string {
	return `private:${peerId}`;
}

export function convKeyGroup(groupId: string): string {
	return `group:${groupId}`;
}

export function loadConvCache(convKey: string): ConvCache | null {
	if (typeof window === 'undefined' || !convKey) return null;
	try {
		const raw = localStorage.getItem(storageKey(convKey));
		if (!raw) return null;
		const parsed = JSON.parse(raw) as ConvCache;
		if (!parsed || !Array.isArray(parsed.messages)) return null;
		return {
			key: convKey,
			max_seq: Number(parsed.max_seq) || 0,
			messages: parsed.messages,
			updated_at: parsed.updated_at || 0
		};
	} catch {
		return null;
	}
}

export function saveConvCache(convKey: string, messages: ChatMessage[], maxSeq?: number): void {
	if (typeof window === 'undefined' || !convKey) return;
	const ordered = sortMessagesBySeq([...messages]);
	const trimmed = ordered.length > MAX_CACHED ? ordered.slice(ordered.length - MAX_CACHED) : ordered;
	const computedMax = Math.max(maxSeq ?? 0, maxSeqOf(trimmed));
	const payload: ConvCache = {
		key: convKey,
		max_seq: computedMax,
		messages: trimmed.map(stripClientOnly),
		updated_at: Date.now()
	};
	try {
		localStorage.setItem(storageKey(convKey), JSON.stringify(payload));
	} catch {
		// Quota exceeded — drop oldest half and retry once.
		try {
			payload.messages = payload.messages.slice(Math.floor(payload.messages.length / 2));
			localStorage.setItem(storageKey(convKey), JSON.stringify(payload));
		} catch {
			// ignore
		}
	}
}

function stripClientOnly(m: ChatMessage): ChatMessage {
	// Drop client-only fields before localStorage persist.
	const rest = { ...m };
	delete rest.send_status;
	delete rest._local_plain;
	return rest;
}

export function maxSeqOf(messages: ChatMessage[]): number {
	let max = 0;
	for (const m of messages) {
		const s = Number(m.seq) || 0;
		if (s > max) max = s;
	}
	return max;
}

/** Lowest positive seq (0 if none). Used as before_seq when loading older pages. */
export function minSeqOf(messages: ChatMessage[]): number {
	let min = 0;
	for (const m of messages) {
		const s = Number(m.seq) || 0;
		if (s <= 0) continue;
		if (min === 0 || s < min) min = s;
	}
	return min;
}

export function minTimestampOf(messages: ChatMessage[]): number {
	let min = 0;
	for (const m of messages) {
		const t = Number(m.timestamp) || 0;
		if (t <= 0) continue;
		if (min === 0 || t < min) min = t;
	}
	return min;
}

/** Remove one conversation cache entry. */
export function clearConvCache(convKey: string): void {
	if (typeof window === 'undefined' || !convKey) return;
	try {
		localStorage.removeItem(storageKey(convKey));
	} catch {
		// ignore
	}
}

/** Remove all chat history caches on this device. */
export function clearAllConvCaches(): number {
	if (typeof window === 'undefined') return 0;
	let n = 0;
	try {
		const keys: string[] = [];
		for (let i = 0; i < localStorage.length; i++) {
			const k = localStorage.key(i);
			if (k && k.startsWith(CACHE_PREFIX)) keys.push(k);
		}
		for (const k of keys) {
			localStorage.removeItem(k);
			n++;
		}
	} catch {
		// ignore
	}
	return n;
}

/** Sort ascending by seq (prefer), then timestamp, then id. */
export function sortMessagesBySeq(messages: ChatMessage[]): ChatMessage[] {
	return messages.sort((a, b) => {
		const sa = Number(a.seq) || 0;
		const sb = Number(b.seq) || 0;
		if (sa > 0 && sb > 0 && sa !== sb) return sa - sb;
		const ta = a.timestamp ?? 0;
		const tb = b.timestamp ?? 0;
		if (ta !== tb) return ta - tb;
		return String(a.id ?? '').localeCompare(String(b.id ?? ''));
	});
}

/**
 * Merge two lists by id (prefer newer / higher seq), return sorted unique list.
 */
export function mergeById(existing: ChatMessage[], incoming: ChatMessage[]): ChatMessage[] {
	const map = new Map<string, ChatMessage>();
	const keyOf = (m: ChatMessage) => {
		if (m.id) return `id:${m.id}`;
		return `f:${m.from}|t:${m.to}|g:${m.group_id ?? ''}|ts:${m.timestamp ?? 0}|c:${m.content_type ?? ''}|${(m.content || '').slice(0, 40)}`;
	};
	for (const m of existing) {
		map.set(keyOf(m), m);
	}
	for (const m of incoming) {
		const k = keyOf(m);
		const prev = map.get(k);
		if (!prev) {
			map.set(k, m);
			continue;
		}
		// Prefer higher seq / keep plaintext local fields
		const preferIncoming = (Number(m.seq) || 0) >= (Number(prev.seq) || 0);
		const merged: ChatMessage = preferIncoming
			? {
					...prev,
					...m,
					content: m.content || prev.content,
					_local_plain: prev._local_plain ?? m._local_plain,
					send_status: prev.send_status === 'sending' ? 'sent' : (m.send_status ?? prev.send_status)
				}
			: {
					...m,
					...prev,
					seq: prev.seq || m.seq,
					_local_plain: prev._local_plain ?? m._local_plain
				};
		map.set(k, merged);
	}
	return sortMessagesBySeq(Array.from(map.values()));
}
