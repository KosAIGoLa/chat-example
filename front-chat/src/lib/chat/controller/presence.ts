/**
 * Presence, unread badges, previews, labels, balance.
 */

import { chatService } from '$lib/api';
import { redPacketService } from '$lib/api/red-packet.service';
import type {
	ChatMessage,
	ChatMode,
	FriendUser,
	GroupMember,
	GroupMembersEvent,
	OnlineUser,
	PresenceEvent
} from '../types';
import { messagePreview } from '../utils';
import { conversationKey } from './message-helpers';
import { normalizeOnlineList, withoutSelf } from './normalize';

export interface PresenceDeps {
	getMyUserId: () => string;
	getChatMode: () => ChatMode;
	getTargetUser: () => string;
	getGroupId: () => string;
	getUserLabels: () => Record<string, string>;
	setUserLabels: (l: Record<string, string>) => void;
	getUnreadPeers: () => Record<string, boolean>;
	setUnreadPeers: (u: Record<string, boolean>) => void;
	getUnreadGroups: () => Record<string, boolean>;
	setUnreadGroups: (u: Record<string, boolean>) => void;
	getLastPreviews: () => Record<string, { text: string; ts: number }>;
	setLastPreviews: (p: Record<string, { text: string; ts: number }>) => void;
	getOnlineUsers: () => OnlineUser[];
	setOnlineUsers: (u: OnlineUser[]) => void;
	getFriends: () => FriendUser[];
	setFriends: (f: FriendUser[]) => void;
	getGroupMembers: () => GroupMember[];
	setGroupMembers: (m: GroupMember[]) => void;
	getBalance: () => number;
	setBalance: (b: number) => void;
	refreshGroupMembers: (g?: string) => Promise<void>;
	onBalanceChange?: (balance: number) => void;
}

export function createPresenceApi(deps: PresenceDeps) {
	function rememberUsers(users: OnlineUser[]) {
		if (!users.length) return;
		const next = { ...deps.getUserLabels() };
		for (const u of users) {
			if (u.user_id && u.username) next[u.user_id] = u.username;
		}
		deps.setUserLabels(next);
	}

	function updatePreview(msg: ChatMessage) {
		const key = conversationKey(msg, deps.getMyUserId());
		if (!key) return;
		const ts = msg.timestamp ?? 0;
		const prev = deps.getLastPreviews()[key];
		if (prev && prev.ts > ts) return;
		deps.setLastPreviews({
			...deps.getLastPreviews(),
			[key]: { text: messagePreview(msg), ts }
		});
	}

	function markUnread(peerId: string) {
		const id = String(peerId ?? '');
		if (!id || id === deps.getMyUserId()) return;
		if (deps.getChatMode() === 'private' && String(deps.getTargetUser()) === id) return;
		const unreadPeers = deps.getUnreadPeers();
		if (unreadPeers[id]) return;
		deps.setUnreadPeers({ ...unreadPeers, [id]: true });
	}

	function clearUnread(peerId: string) {
		const id = String(peerId ?? '');
		if (!id) return;
		const unreadPeers = deps.getUnreadPeers();
		if (!unreadPeers[id]) return;
		const next = { ...unreadPeers };
		delete next[id];
		deps.setUnreadPeers(next);
	}

	function hasUnread(peerId: string): boolean {
		return !!deps.getUnreadPeers()[String(peerId ?? '')];
	}

	function markGroupUnread(gid: string) {
		const id = String(gid ?? '');
		if (!id) return;
		if (deps.getChatMode() === 'group' && String(deps.getGroupId()) === id) return;
		const unreadGroups = deps.getUnreadGroups();
		if (unreadGroups[id]) return;
		deps.setUnreadGroups({ ...unreadGroups, [id]: true });
	}

	function clearGroupUnread(gid: string) {
		const id = String(gid ?? '');
		const unreadGroups = deps.getUnreadGroups();
		if (!id || !unreadGroups[id]) return;
		const next = { ...unreadGroups };
		delete next[id];
		deps.setUnreadGroups(next);
	}

	function hasGroupUnread(gid: string): boolean {
		return !!deps.getUnreadGroups()[String(gid ?? '')];
	}

	async function refreshBalance() {
		try {
			const w = await redPacketService.getWallet();
			const bal = w.balance ?? 0;
			deps.setBalance(bal);
			deps.onBalanceChange?.(bal);
		} catch {
			// ignore
		}
	}

	function ensurePeerListed(peerId: string, username?: string) {
		if (!peerId || peerId === deps.getMyUserId()) return;
		const labels = deps.getUserLabels();
		const name = username?.trim() || labels[peerId] || peerId;
		rememberUsers([{ user_id: peerId, username: name }]);
	}

	function applyPresence(event: PresenceEvent) {
		const uid = String(event.user_id ?? '');
		if (!uid) return;
		const isOnline = event.online === true;
		const labels = deps.getUserLabels();
		const name = (event.username && event.username.trim()) || labels[uid] || uid;

		const friends = deps.getFriends();
		if (friends.some((f) => f.user_id === uid)) {
			deps.setFriends(
				friends.map((f) =>
					f.user_id === uid
						? { ...f, online: isOnline, username: name || f.username }
						: f
				)
			);
		}

		const groupMembers = deps.getGroupMembers();
		if (groupMembers.some((m) => m.user_id === uid)) {
			deps.setGroupMembers(
				groupMembers.map((m) =>
					m.user_id === uid
						? {
								...m,
								online: isOnline,
								username: name || m.username || uid
							}
						: m
				)
			);
		}

		if (uid === deps.getMyUserId()) return;

		if (isOnline) {
			rememberUsers([{ user_id: uid, username: name }]);
			const onlineUsers = deps.getOnlineUsers();
			const idx = onlineUsers.findIndex((u) => u.user_id === uid);
			if (idx >= 0) {
				const next = [...onlineUsers];
				next[idx] = { user_id: uid, username: name };
				deps.setOnlineUsers(next);
			} else {
				deps.setOnlineUsers([...onlineUsers, { user_id: uid, username: name }]);
			}
		} else {
			deps.setOnlineUsers(deps.getOnlineUsers().filter((u) => u.user_id !== uid));
		}
	}

	function applyGroupMembers(event: GroupMembersEvent) {
		const gid = String(event.group_id ?? '');
		if (!gid || gid !== deps.getGroupId()) return;
		void deps.refreshGroupMembers(gid);
	}

	async function refreshOnlineUsers() {
		try {
			const res = await chatService.getOnlineUsers();
			const raw = res.online_users as unknown;
			const myUserId = deps.getMyUserId();
			// Support legacy bare-id arrays from older servers.
			let list: OnlineUser[];
			if (Array.isArray(raw) && raw.some((x) => typeof x === 'string')) {
				list = [];
				for (const item of raw as unknown[]) {
					if (typeof item === 'string') {
						list.push({ user_id: item, username: item });
					} else if (item && typeof item === 'object') {
						const o = item as Record<string, unknown>;
						const uid = String(o.user_id ?? o.id ?? '');
						if (!uid) continue;
						list.push({
							user_id: uid,
							username: String(o.username ?? o.name ?? uid) || uid
						});
					}
				}
				list = withoutSelf(list, myUserId);
			} else {
				list = withoutSelf(normalizeOnlineList(raw), myUserId);
			}
			deps.setOnlineUsers(list);
			rememberUsers(list);
		} catch {
			// ignore
		}
	}

	function displayName(userId: string): string {
		if (!userId) return '';
		return deps.getUserLabels()[userId] || userId;
	}

	return {
		rememberUsers,
		updatePreview,
		markUnread,
		clearUnread,
		hasUnread,
		markGroupUnread,
		clearGroupUnread,
		hasGroupUnread,
		ensurePeerListed,
		applyPresence,
		applyGroupMembers,
		refreshOnlineUsers,
		displayName,
		refreshBalance
	};
}

export type PresenceApi = ReturnType<typeof createPresenceApi>;
