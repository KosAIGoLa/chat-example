/**
 * Conversation navigation: join/leave/select group or private peer, mode switch,
 * create/dissolve group, and WS onSocketReady rejoin + history reload.
 */

import { groupService } from '$lib/api';
import { toastError } from '$lib/ui/notify.svelte';
import type { ChatMessage, ChatMode, GroupInfo } from '../types';
import type { ChatState } from './create-chat-state.svelte';
import { appendUnique, saveJoinedGroups } from './joined-groups';
import type { WiredChatController } from './wire-controller';

export function createConversationNav(deps: {
	state: ChatState;
	myUserId: string;
	wired: Pick<
		WiredChatController,
		| 'isWsOpen'
		| 'wsSession'
		| 'notifyTypingStop'
		| 'clearUnread'
		| 'clearGroupUnread'
		| 'clearTypingForConversation'
		| 'rememberUsers'
		| 'ensurePeerListed'
		| 'loadPrivateHistory'
		| 'loadGroupHistory'
		| 'reloadActiveHistory'
		| 'refreshGroupMembers'
		| 'refreshMyGroups'
		| 'refreshAnnouncements'
		| 'refreshGroupMeeting'
		| 'refreshOnlineUsers'
		| 'refreshFriends'
		| 'refreshIncomingRequests'
		| 'refreshBlacklist'
		| 'refreshBalance'
		| 'flushPendingSends'
		| 'createGroupInner'
		| 'dissolveGroupInner'
		| 'isGroupOwner'
	>;
}) {
	const { state: s, myUserId, wired: w } = deps;

	async function onSocketReady(socket: WebSocket, gen: number) {
		if (!w.wsSession.isCurrentSocket(socket, gen)) return;

		void w.refreshOnlineUsers();
		void w.refreshFriends();
		void w.refreshIncomingRequests();
		void w.refreshBlacklist();
		void w.refreshMyGroups();
		void w.refreshBalance();

		for (const g of s.joinedGroups) {
			if (!w.wsSession.isCurrentSocket(socket, gen) || socket.readyState !== WebSocket.OPEN) {
				return;
			}
			try {
				await w.wsSession.wsSendJSON(
					{
						type: 'join_group',
						from: myUserId,
						to: g,
						content: 'rejoin',
						group_id: g
					} satisfies ChatMessage,
					socket
				);
			} catch (err) {
				console.error('[ws] rejoin group failed', g, err);
			}
			void groupService.join(g, { rejoin: true }).catch(() => {
				// REST join needs WS online; ignore race — WS join_group still applies.
			});
		}

		if (!w.wsSession.isCurrentSocket(socket, gen)) return;
		void w.reloadActiveHistory();
		if (s.chatMode === 'group' && s.groupId.trim()) {
			const gid = s.groupId.trim();
			void w.refreshGroupMembers(gid);
			void w.refreshAnnouncements(gid);
			// Catch-up open meeting after reconnect (no polling).
			void w.refreshGroupMeeting(gid);
		} else if (s.chatMode === 'private' && s.targetUser.trim()) {
			void w.refreshAnnouncements(s.targetUser.trim());
		}
		void w.flushPendingSends();
	}

	async function joinGroup() {
		const g = s.groupId.trim();
		if (!g) return;
		try {
			await groupService.join(g);
			s.joinedGroups = appendUnique(s.joinedGroups, g);
			saveJoinedGroups(s.joinedGroups);
			if (w.isWsOpen()) {
				await w.wsSession.wsSendJSON({
					type: 'join_group',
					from: myUserId,
					to: g,
					content: '',
					group_id: g
				} satisfies ChatMessage);
			}
			s.chatMode = 'group';
			s.groupId = g;
			await Promise.all([
				w.loadGroupHistory(g),
				w.refreshGroupMembers(g),
				w.refreshGroupMeeting(g),
				w.refreshMyGroups()
			]);
		} catch (err) {
			toastError((err as Error).message || '加入群失败');
		}
	}

	async function leaveGroup(g: string) {
		try {
			await groupService.leave(g);
			s.joinedGroups = s.joinedGroups.filter((g2) => g2 !== g);
			saveJoinedGroups(s.joinedGroups);
			const nextMeta = { ...s.groupMeta };
			delete nextMeta[g];
			s.groupMeta = nextMeta;
			if (w.isWsOpen()) {
				await w.wsSession.wsSendJSON({
					type: 'leave_group',
					from: myUserId,
					to: g,
					content: '',
					group_id: g
				} satisfies ChatMessage);
			}
			if (s.chatMode === 'group' && s.groupId === g) {
				s.messages = [];
				s.groupId = '';
				s.groupMembers = [];
			}
		} catch (err) {
			toastError((err as Error).message || '退出群失败');
		}
	}

	async function selectPrivateUser(userId: string, username?: string) {
		w.clearUnread(userId);
		const peer = String(userId ?? '').trim();
		if (!peer || peer === myUserId) return;

		w.notifyTypingStop();
		if (s.targetUser !== peer || s.chatMode !== 'private') {
			s.replyTarget = null;
			s.selectMode = false;
			s.selectedMsgIds = [];
			s.groupAnnouncements = [];
		}
		s.chatMode = 'private';
		s.targetUser = peer;
		w.clearUnread(peer);
		w.clearTypingForConversation();

		if (username) {
			w.rememberUsers([{ user_id: peer, username }]);
		}
		w.ensurePeerListed(peer, username);

		await Promise.all([w.loadPrivateHistory(peer), w.refreshAnnouncements(peer)]);
		w.clearUnread(peer);
	}

	async function selectGroup(g: string) {
		w.notifyTypingStop();
		if (s.groupId !== g) {
			s.replyTarget = null;
			s.selectMode = false;
			s.selectedMsgIds = [];
			s.groupAnnouncements = [];
		}
		s.chatMode = 'group';
		s.groupId = g;
		s.groupMembers = [];
		w.clearGroupUnread(g);
		w.clearTypingForConversation();
		const firstJoin = !s.joinedGroups.includes(g);
		if (firstJoin) {
			try {
				await groupService.join(g);
				s.joinedGroups = appendUnique(s.joinedGroups, g);
				saveJoinedGroups(s.joinedGroups);
			} catch {
				// still try to load history
			}
		}
		if (w.isWsOpen()) {
			try {
				await w.wsSession.wsSendJSON({
					type: 'join_group',
					from: myUserId,
					to: g,
					content: firstJoin ? '' : 'rejoin',
					group_id: g
				} satisfies ChatMessage);
			} catch (err) {
				console.error('[ws] join_group send failed', err);
			}
		}
		await Promise.all([
			w.loadGroupHistory(g),
			w.refreshGroupMembers(g),
			w.refreshGroupMeeting(g),
			w.refreshAnnouncements(g)
		]);
	}

	function setChatMode(mode: ChatMode) {
		if (mode !== s.chatMode) {
			s.replyTarget = null;
			s.selectMode = false;
			s.selectedMsgIds = [];
			s.groupAnnouncements = [];
		}
		if (s.chatMode === mode) return;
		w.notifyTypingStop();
		s.chatMode = mode;
		s.messages = [];
		s.loadedKey = '';
		w.clearTypingForConversation();
		if (mode === 'group' && s.groupId.trim()) {
			void w.refreshGroupMembers(s.groupId.trim());
			void w.refreshAnnouncements(s.groupId.trim());
		} else if (mode === 'private') {
			s.groupMembers = [];
			if (s.targetUser.trim()) {
				void w.refreshAnnouncements(s.targetUser.trim());
			} else {
				s.groupAnnouncements = [];
			}
		}
		void w.reloadActiveHistory();
	}

	async function createGroup(name?: string, customId?: string): Promise<GroupInfo> {
		const g = await w.createGroupInner(name, customId);
		if (w.isWsOpen()) {
			try {
				await w.wsSession.wsSendJSON({
					type: 'join_group',
					from: myUserId,
					to: g.id,
					content: 'rejoin',
					group_id: g.id
				} satisfies ChatMessage);
			} catch {
				// hub already joined on create
			}
		}
		s.chatMode = 'group';
		s.groupId = g.id;
		await Promise.all([w.loadGroupHistory(g.id), w.refreshGroupMembers(g.id)]);
		return g;
	}

	async function dissolveGroup(g: string) {
		const id = g.trim();
		if (!id) return;
		if (!w.isGroupOwner(id)) {
			throw new Error('仅群主可以解散群');
		}
		await w.dissolveGroupInner(id);
	}

	return {
		onSocketReady,
		joinGroup,
		leaveGroup,
		selectPrivateUser,
		selectGroup,
		setChatMode,
		createGroup,
		dissolveGroup
	};
}

export type ConversationNav = ReturnType<typeof createConversationNav>;
