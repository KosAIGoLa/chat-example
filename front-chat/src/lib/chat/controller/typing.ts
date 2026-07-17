/**
 * Typing indicators: inbound display + outbound start/stop pings.
 */

import type { ChatMode, TypingEvent, TypingUser } from '../types';
import {
	activeConvKey,
	clearTypingHint,
	setTypingHint,
	typingUI
} from '../typing-ui.svelte';
import { TYPING_IDLE_MS, TYPING_PING_MS, TYPING_TTL_MS } from './constants';

export interface TypingDeps {
	getMyUserId: () => string;
	getChatMode: () => ChatMode;
	setChatMode: (m: ChatMode) => void;
	getTargetUser: () => string;
	setTargetUser: (v: string) => void;
	getGroupId: () => string;
	setGroupId: (v: string) => void;
	getJoinedGroups: () => string[];
	getTypingUsers: () => TypingUser[];
	setTypingUsers: (u: TypingUser[]) => void;
	getTypingHint: () => string;
	setTypingHint: (h: string) => void;
	getUserLabel: (userId: string) => string | undefined;
	rememberUsers: (users: { user_id: string; username: string }[]) => void;
	isWsOpen: () => boolean;
	wsSendJSON: (payload: unknown) => Promise<void>;
	onTypingHintChange?: (label: string) => void;
}

export function createTypingApi(deps: TypingDeps) {
	/** Outbound typing state machine private state. */
	let typingActive = false;
	let lastTypingPingAt = 0;
	let typingIdleTimer: ReturnType<typeof setTimeout> | null = null;
	let typingPruneTimer: ReturnType<typeof setInterval> | null = null;
	let lastTypingSession: { mode: ChatMode; peer: string; group: string } | null = null;

	function currentConvKey(): string {
		return activeConvKey(deps.getChatMode(), deps.getTargetUser(), deps.getGroupId());
	}

	function formatTypingLabel(list: TypingUser[]): string {
		if (list.length === 0) return '';
		const names = list.map((t) => t.username || t.user_id);
		if (deps.getChatMode() === 'private' || (names.length === 1 && !deps.getGroupId())) {
			return `${names[0] || '对方'} 正在输入…`;
		}
		if (names.length === 1) return `${names[0]} 正在输入…`;
		if (names.length === 2) return `${names[0]}、${names[1]} 正在输入…`;
		return `${names[0]} 等 ${names.length} 人正在输入…`;
	}

	function publishTypingHint(list: TypingUser[] = deps.getTypingUsers()) {
		const label = list.length === 0 ? '' : formatTypingLabel(list);
		deps.setTypingHint(label);
		if (label) {
			setTypingHint(label, currentConvKey());
		} else {
			clearTypingHint();
		}
		deps.onTypingHintChange?.(label);
	}

	function pruneTypingUsers() {
		const now = Date.now();
		const typingUsers = deps.getTypingUsers();
		const next = typingUsers.filter((t) => t.until > now);
		if (next.length !== typingUsers.length) {
			deps.setTypingUsers(next);
			publishTypingHint(next);
		} else if (typingUsers.length === 0 && (deps.getTypingHint() || typingUI.hint)) {
			publishTypingHint([]);
		}
	}

	function ensureTypingPrune() {
		if (typingPruneTimer != null) return;
		typingPruneTimer = setInterval(pruneTypingUsers, 400);
	}

	function removeRemoteTyper(userId: string) {
		const id = String(userId ?? '');
		if (!id) return;
		const typingUsers = deps.getTypingUsers();
		const before = typingUsers.length;
		const next = typingUsers.filter((t) => t.user_id !== id);
		if (next.length !== before) {
			deps.setTypingUsers(next);
			publishTypingHint(next);
		}
	}

	function applyTypingEvent(ev: TypingEvent) {
		const from = String(ev.from ?? '');
		if (!from || from === deps.getMyUserId()) return;

		const action = String(ev.content || 'start').toLowerCase();
		const isStop = action === 'stop' || action === '0' || action === 'false';

		if (isStop) {
			removeRemoteTyper(from);
			return;
		}

		const evGroup = String(ev.group_id ?? '').trim();
		const activeGroup = String(deps.getGroupId() ?? '').trim();
		const activePeer = String(deps.getTargetUser() ?? '').trim();

		if (evGroup) {
			if (deps.getChatMode() !== 'group') return;
			if (activeGroup && activeGroup !== evGroup) return;
			if (!activeGroup && !deps.getJoinedGroups().includes(evGroup)) return;
			if (!activeGroup) deps.setGroupId(evGroup);
		} else {
			if (deps.getChatMode() !== 'private') return;
			if (!activePeer || activePeer !== from) return;
		}

		const name =
			(ev.from_name || deps.getUserLabel(from) || from).trim() || from;
		const until = Date.now() + TYPING_TTL_MS;
		const rest = deps.getTypingUsers().filter((t) => t.user_id !== from);
		const next = [...rest, { user_id: from, username: name, until }];
		deps.setTypingUsers(next);
		publishTypingHint(next);
		ensureTypingPrune();
		if (name && from) {
			deps.rememberUsers([{ user_id: from, username: name }]);
		}
	}

	function clearTypingForConversation() {
		deps.setTypingUsers([]);
		deps.setTypingHint('');
		clearTypingHint();
		deps.onTypingHintChange?.('');
	}

	function typingLabel(): string {
		pruneTypingUsers();
		return deps.getTypingHint() || formatTypingLabel(deps.getTypingUsers()) || typingUI.hint;
	}

	function resolveTypingSession(session?: {
		mode?: ChatMode;
		peer?: string;
		group?: string;
	}): { mode: ChatMode; peer: string; group: string } | null {
		let mode: ChatMode = session?.mode ?? deps.getChatMode();
		const peer = (session?.peer ?? deps.getTargetUser()).trim();
		const group = (session?.group ?? deps.getGroupId()).trim();

		if ((session?.mode === 'group' || deps.getChatMode() === 'group') && group) {
			mode = 'group';
		} else if (group && !peer) {
			mode = 'group';
		} else if (peer) {
			mode = 'private';
		}

		if (mode === 'private' && !peer) return null;
		if (mode === 'group' && !group) return null;
		return { mode, peer, group };
	}

	function notifyTyping(session?: { mode?: ChatMode; peer?: string; group?: string }) {
		const resolved = resolveTypingSession(session);
		if (!resolved) return;
		if (!deps.isWsOpen()) return;

		const { mode, peer, group } = resolved;
		deps.setChatMode(mode);
		if (peer) deps.setTargetUser(peer);
		if (group) deps.setGroupId(group);

		lastTypingSession = { mode, peer, group };
		typingActive = true;

		const now = Date.now();
		const shouldPing = lastTypingPingAt === 0 || now - lastTypingPingAt >= TYPING_PING_MS;
		if (shouldPing) {
			lastTypingPingAt = now;
			const payload =
				mode === 'private'
					? {
							type: 'typing' as const,
							from: deps.getMyUserId(),
							to: peer,
							content: 'start',
							timestamp: Math.floor(now / 1000)
						}
					: {
							type: 'typing' as const,
							from: deps.getMyUserId(),
							to: group,
							group_id: group,
							content: 'start',
							timestamp: Math.floor(now / 1000)
						};
			void deps.wsSendJSON(payload).catch((err) => {
				console.warn('[typing] start failed', err);
			});
		}

		if (typingIdleTimer != null) clearTimeout(typingIdleTimer);
		typingIdleTimer = setTimeout(() => {
			typingIdleTimer = null;
			notifyTypingStop(lastTypingSession ?? resolved);
		}, TYPING_IDLE_MS);
	}

	function notifyTypingStop(
		session?: { mode?: ChatMode; peer?: string; group?: string } | null
	) {
		if (typingIdleTimer != null) {
			clearTimeout(typingIdleTimer);
			typingIdleTimer = null;
		}

		if (!typingActive) {
			lastTypingPingAt = 0;
			return;
		}
		typingActive = false;
		lastTypingPingAt = 0;

		const resolved =
			resolveTypingSession(session ?? lastTypingSession ?? undefined) ?? lastTypingSession;
		lastTypingSession = null;
		if (!resolved) return;
		if (!deps.isWsOpen()) return;

		const { mode, peer, group } = resolved;
		const payload =
			mode === 'private'
				? {
						type: 'typing' as const,
						from: deps.getMyUserId(),
						to: peer,
						content: 'stop',
						timestamp: Math.floor(Date.now() / 1000)
					}
				: {
						type: 'typing' as const,
						from: deps.getMyUserId(),
						to: group,
						group_id: group,
						content: 'stop',
						timestamp: Math.floor(Date.now() / 1000)
					};
		void deps.wsSendJSON(payload).catch((err) => {
			console.warn('[typing] stop failed', err);
		});
	}

	return {
		formatTypingLabel,
		publishTypingHint,
		pruneTypingUsers,
		ensureTypingPrune,
		removeRemoteTyper,
		applyTypingEvent,
		clearTypingForConversation,
		typingLabel,
		resolveTypingSession,
		notifyTyping,
		notifyTypingStop
	};
}

export type TypingApi = ReturnType<typeof createTypingApi>;
