/**
 * Friends, friend requests, blacklist (block ≠ remove friend).
 */

import { friendService } from '$lib/api';
import type { BlacklistUser, FriendRequest, FriendUser, OnlineUser } from '../types';
import { clearConvCache, convKeyPrivate } from '../message-cache';

export interface FriendsDeps {
	getMyUserId: () => string;
	getFriends: () => FriendUser[];
	setFriends: (f: FriendUser[]) => void;
	getIncoming: () => FriendRequest[];
	setIncoming: (r: FriendRequest[]) => void;
	getBlacklist: () => BlacklistUser[];
	setBlacklist: (b: BlacklistUser[]) => void;
	getChatMode: () => string;
	getTargetUser: () => string;
	setTargetUser: (v: string) => void;
	setMessages: (m: never[] | unknown) => void;
	setLoadedKey: (k: string) => void;
	/** Clear active pins when private conv ends. */
	setGroupAnnouncements?: (a: []) => void;
	getLastPreviews: () => Record<string, { text: string; ts: number }>;
	setLastPreviews: (p: Record<string, { text: string; ts: number }>) => void;
	getUnreadPeers: () => Record<string, boolean>;
	setUnreadPeers: (u: Record<string, boolean>) => void;
	rememberUsers: (users: OnlineUser[]) => void;
	filterGroupMessagesFromUser?: (uid: string) => void;
}

export function createFriendsApi(deps: FriendsDeps) {
	async function refreshFriends() {
		try {
			const res = await friendService.listFriends();
			const fromAPI = res.friends ?? [];
			deps.setFriends(
				fromAPI.map((f) => ({
					...f,
					online: !!f.online
				}))
			);
			for (const f of deps.getFriends()) {
				deps.rememberUsers([{ user_id: f.user_id, username: f.username }]);
			}
		} catch {
			// ignore
		}
	}

	async function refreshIncomingRequests() {
		try {
			const res = await friendService.listIncoming();
			deps.setIncoming(res.requests ?? []);
		} catch {
			// ignore
		}
	}

	async function inviteFriend(username: string) {
		const req = await friendService.invite({ username });
		await refreshIncomingRequests();
		return req;
	}

	async function acceptFriendRequest(id: number) {
		await friendService.accept(id);
		await Promise.all([refreshFriends(), refreshIncomingRequests()]);
	}

	async function rejectFriendRequest(id: number) {
		await friendService.reject(id);
		deps.setIncoming(deps.getIncoming().filter((r) => r.id !== id));
	}

	/** 解除好友：清空双方私聊历史（服务端 + 本机）。 */
	async function removeFriend(userId: string) {
		const uid = String(userId ?? '').trim();
		if (!uid) return;
		await friendService.remove(uid);
		deps.setFriends(deps.getFriends().filter((f) => f.user_id !== uid));
		clearConvCache(convKeyPrivate(uid));
		const nextPrev = { ...deps.getLastPreviews() };
		delete nextPrev[`private:${uid}`];
		deps.setLastPreviews(nextPrev);
		const nextUnread = { ...deps.getUnreadPeers() };
		delete nextUnread[uid];
		deps.setUnreadPeers(nextUnread);
		if (deps.getChatMode() === 'private' && deps.getTargetUser() === uid) {
			deps.setMessages([]);
			deps.setTargetUser('');
			deps.setLoadedKey('');
			deps.setGroupAnnouncements?.([]);
		}
	}

	async function refreshBlacklist() {
		try {
			const res = await friendService.listBlacklist();
			deps.setBlacklist(res.blacklist ?? []);
			for (const u of deps.getBlacklist()) {
				deps.rememberUsers([{ user_id: u.user_id, username: u.username }]);
			}
		} catch {
			// ignore
		}
	}

	/** 拉黑：保留好友关系；列表隐藏；屏蔽消息。 */
	async function blockUser(opts: { user_id?: string; username?: string }) {
		const entry = await friendService.block(opts);
		const uid = entry.user_id;
		deps.setFriends(deps.getFriends().filter((f) => f.user_id !== uid));
		deps.setIncoming(
			deps.getIncoming().filter((r) => r.from_user_id !== uid && r.to_user_id !== uid)
		);
		await refreshBlacklist();
		if (deps.getChatMode() === 'private' && deps.getTargetUser() === uid) {
			deps.setMessages([]);
			deps.setTargetUser('');
			deps.setGroupAnnouncements?.([]);
		} else if (deps.getChatMode() === 'group') {
			deps.filterGroupMessagesFromUser?.(uid);
		}
		return entry;
	}

	async function unblockUser(userId: string) {
		await friendService.unblock(userId);
		deps.setBlacklist(deps.getBlacklist().filter((u) => u.user_id !== userId));
		await refreshFriends();
	}

	function isUserBlocked(userId: string): boolean {
		const uid = String(userId ?? '').trim();
		if (!uid) return false;
		return deps.getBlacklist().some((b) => b.user_id === uid);
	}

	return {
		refreshFriends,
		refreshIncomingRequests,
		inviteFriend,
		acceptFriendRequest,
		rejectFriendRequest,
		removeFriend,
		refreshBlacklist,
		blockUser,
		unblockUser,
		isUserBlocked
	};
}

export type FriendsApi = ReturnType<typeof createFriendsApi>;
