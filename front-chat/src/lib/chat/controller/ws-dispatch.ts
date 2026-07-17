/**
 * Top-level WebSocket frame dispatcher for chat domain events.
 */

import type {
	CallEvent,
	ChatMessage,
	ChatMode,
	EditEvent,
	FriendEvent,
	GroupAnnouncement,
	GroupAnnouncementEvent,
	PrivatePinEvent,
	GroupDissolvedEvent,
	GroupInfo,
	GroupMember,
	GroupMembersEvent,
	MeetingEvent,
	OfflineSyncEvent,
	PresenceEvent,
	RecallEvent,
	RedPacketClaimedEvent,
	TypingEvent
} from '../types';
import { clearConvCache, convKeyPrivate } from '../message-cache';
import { isChatContent } from './message-helpers';
import { saveJoinedGroups } from './joined-groups';

export interface WsDispatchDeps {
	getMyUserId: () => string;
	getChatMode: () => ChatMode;
	getTargetUser: () => string;
	setTargetUser: (v: string) => void;
	getGroupId: () => string;
	setGroupId: (v: string) => void;
	getMessages: () => ChatMessage[];
	setMessages: (m: ChatMessage[]) => void;
	getLastPreviews: () => Record<string, { text: string; ts: number }>;
	setLastPreviews: (p: Record<string, { text: string; ts: number }>) => void;
	getUnreadPeers: () => Record<string, boolean>;
	setUnreadPeers: (u: Record<string, boolean>) => void;
	getJoinedGroups: () => string[];
	setJoinedGroups: (g: string[]) => void;
	getGroupMeta: () => Record<string, GroupInfo>;
	setGroupMeta: (m: Record<string, GroupInfo>) => void;
	setGroupMembers: (m: GroupMember[]) => void;
	getGroupAnnouncements: () => GroupAnnouncement[];
	setGroupAnnouncements: (a: GroupAnnouncement[]) => void;
	setLoadedKey: (k: string) => void;
	applyRecall: (id: string) => void;
	applyEditEvent: (ev: EditEvent) => void | Promise<void>;
	applyMeetingEvent: (ev: MeetingEvent) => void;
	applyPresence: (ev: PresenceEvent) => void;
	applyGroupMembers: (ev: GroupMembersEvent) => void;
	applyTypingEvent: (ev: TypingEvent) => void;
	handleIncomingChat: (msg: ChatMessage) => void | Promise<void>;
	refreshIncomingRequests: () => void | Promise<void>;
	refreshFriends: () => void | Promise<void>;
	refreshBlacklist: () => void | Promise<void>;
	refreshOnlineUsers: () => void | Promise<void>;
	refreshAnnouncements: (scopeId?: string) => void | Promise<void>;
	refreshBalance: () => void | Promise<void>;
	onCallEvent?: (ev: CallEvent) => void;
	onMeetingEvent?: (ev: MeetingEvent) => void;
	onRedPacketClaimed?: (ev: RedPacketClaimedEvent) => void;
}

export function createWsDispatcher(deps: WsDispatchDeps) {
	async function handleWsMessage(raw: unknown) {
		const msg = raw as { type?: string } & Record<string, unknown>;
		if (!msg || typeof msg !== 'object' || !msg.type) return;

		// pong handled inside createWsSession
		if (msg.type === 'recall' && 'id' in msg) {
			deps.applyRecall((msg as unknown as RecallEvent).id);
			return;
		}
		if (msg.type === 'edit' && 'id' in msg) {
			void deps.applyEditEvent(msg as unknown as EditEvent);
			return;
		}
		if (msg.type === 'call') {
			deps.onCallEvent?.(msg as unknown as CallEvent);
			return;
		}
		if (msg.type === 'meeting') {
			const me = msg as unknown as MeetingEvent;
			deps.applyMeetingEvent(me);
			deps.onMeetingEvent?.(me);
			return;
		}
		if (msg.type === 'friend_event') {
			const fe = msg as unknown as FriendEvent;
			const myUserId = deps.getMyUserId();
			if (fe.action === 'request') {
				void deps.refreshIncomingRequests();
			} else if (fe.action === 'accepted') {
				void deps.refreshFriends();
				void deps.refreshIncomingRequests();
			} else if (fe.action === 'rejected') {
				void deps.refreshIncomingRequests();
			} else if (fe.action === 'removed') {
				void deps.refreshFriends();
				void deps.refreshIncomingRequests();
				const peer = fe.from_user_id === myUserId ? fe.to_user_id : fe.from_user_id;
				if (peer) {
					clearConvCache(convKeyPrivate(String(peer)));
					const nextPrev = { ...deps.getLastPreviews() };
					delete nextPrev[`private:${peer}`];
					deps.setLastPreviews(nextPrev);
					const nextUnread = { ...deps.getUnreadPeers() };
					delete nextUnread[peer];
					deps.setUnreadPeers(nextUnread);
					if (deps.getChatMode() === 'private' && deps.getTargetUser() === peer) {
						deps.setMessages([]);
						deps.setTargetUser('');
						deps.setLoadedKey('');
						deps.setGroupAnnouncements([]);
					}
				}
			} else if (fe.action === 'blocked') {
				void deps.refreshFriends();
				void deps.refreshIncomingRequests();
				void deps.refreshBlacklist();
				const peer = fe.from_user_id === myUserId ? fe.to_user_id : fe.from_user_id;
				if (
					peer &&
					fe.from_user_id === myUserId &&
					deps.getChatMode() === 'private' &&
					deps.getTargetUser() === peer
				) {
					deps.setMessages([]);
					deps.setTargetUser('');
				}
			} else if (fe.action === 'unblocked') {
				void deps.refreshFriends();
				void deps.refreshBlacklist();
			}
			return;
		}
		if (msg.type === 'error') {
			const err = msg as { message?: string };
			if (err.message) console.warn('[ws] error:', err.message);
			return;
		}
		if (msg.type === 'presence' && 'user_id' in msg) {
			deps.applyPresence(msg as unknown as PresenceEvent);
			void deps.refreshFriends();
			void deps.refreshOnlineUsers();
			return;
		}
		if (msg.type === 'group_announcement' && 'group_id' in msg) {
			const ae = msg as unknown as GroupAnnouncementEvent;
			const gid = String(ae.group_id ?? '');
			if (gid && deps.getChatMode() === 'group' && deps.getGroupId() === gid) {
				if (ae.action === 'remove' && ae.message_id) {
					deps.setGroupAnnouncements(
						deps.getGroupAnnouncements().filter((a) => a.message_id !== ae.message_id)
					);
				} else {
					void deps.refreshAnnouncements(gid);
				}
			}
			return;
		}
		if (msg.type === 'private_pin' && 'peer_id' in msg) {
			const pe = msg as unknown as PrivatePinEvent;
			const peer = String(pe.peer_id ?? '');
			if (peer && deps.getChatMode() === 'private' && deps.getTargetUser() === peer) {
				if (pe.action === 'remove' && pe.message_id) {
					deps.setGroupAnnouncements(
						deps.getGroupAnnouncements().filter((a) => a.message_id !== pe.message_id)
					);
				} else {
					void deps.refreshAnnouncements(peer);
				}
			}
			return;
		}
		if (msg.type === 'group_dissolved' && 'group_id' in msg) {
			const ge = msg as unknown as GroupDissolvedEvent;
			const gid = String(ge.group_id ?? '');
			if (gid) {
				const joined = deps.getJoinedGroups().filter((g) => g !== gid);
				deps.setJoinedGroups(joined);
				saveJoinedGroups(joined);
				const nextMeta = { ...deps.getGroupMeta() };
				delete nextMeta[gid];
				deps.setGroupMeta(nextMeta);
				if (deps.getChatMode() === 'group' && deps.getGroupId() === gid) {
					deps.setMessages([]);
					deps.setGroupId('');
					deps.setGroupMembers([]);
				}
			}
			return;
		}
		if (msg.type === 'group_members' && 'group_id' in msg) {
			deps.applyGroupMembers(msg as unknown as GroupMembersEvent);
			return;
		}
		if (msg.type === 'offline_sync') {
			const oe = msg as unknown as OfflineSyncEvent;
			console.info('[ws] offline sync', oe.count);
			void deps.refreshBalance();
			return;
		}
		if (msg.type === 'typing') {
			deps.applyTypingEvent(msg as unknown as TypingEvent);
			return;
		}
		if (msg.type === 'red_packet_claimed') {
			const ev = msg as unknown as RedPacketClaimedEvent;
			deps.onRedPacketClaimed?.(ev);
			if (ev.user_id === deps.getMyUserId()) {
				void deps.refreshBalance();
			}
			return;
		}

		const chat = msg as unknown as ChatMessage;
		if (chat.recalled && chat.id) {
			deps.applyRecall(chat.id);
			return;
		}
		if (!isChatContent(chat) && !chat.recalled) return;
		void deps.handleIncomingChat(chat);
	}

	return { handleWsMessage };
}

export type WsDispatcher = ReturnType<typeof createWsDispatcher>;
