/**
 * Wire domain APIs + WS session against a shared ChatState bag.
 * Forward refs for isWsOpen / refreshGroupMembers / displayName / groupDisplayName.
 */

import type {
	CallEvent,
	ChatMessage,
	MeetingEvent,
	RedPacketClaimedEvent
} from '../types';
import { hasMessageKey } from '../crypto';
import type { ChatState } from './create-chat-state.svelte';
import { createPresenceApi } from './presence';
import { createTypingApi } from './typing';
import { createHistoryApi } from './history';
import { createFriendsApi } from './friends';
import { createMessagingApi } from './messaging';
import { createGroupsApi } from './groups';
import { createMeetingsApi } from './meetings';
import { createWsDispatcher } from './ws-dispatch';
import { createWsSession } from './ws-session';

export type WireControllerOpts = {
	token: string;
	userId: string;
	onUnauthorized?: () => void;
	onCallEvent?: (ev: CallEvent) => void;
	onMeetingEvent?: (ev: MeetingEvent) => void;
	onBalanceChange?: (balance: number) => void;
	onRedPacketClaimed?: (ev: RedPacketClaimedEvent) => void;
	onTypingHintChange?: (label: string) => void;
};

export function wireChatController(ctx: {
	state: ChatState;
	opts: WireControllerOpts;
	ensureCryptoKey: () => Promise<void>;
	/** Called when WS opens; set after conversation-nav is built. */
	onSocketReady: (socket: WebSocket, gen: number) => void | Promise<void>;
}) {
	const { state: s, opts, ensureCryptoKey, onSocketReady } = ctx;
	const myUserId = opts.userId;

	// Forward refs for circular deps between domain modules and WS session.
	let refreshGroupMembersRef = async (_g?: string) => {};
	let displayNameRef = (uid: string) => uid;
	let groupDisplayNameRef = (gid: string) => gid;
	let isWsOpenRef = () => false;
	let wsSendJSONRef = async (_payload: unknown, _socket?: WebSocket) => {};
	let wsSendReliableRef = async (
		_payload: unknown,
		_sendOpts?: { attempts?: number; label?: string }
	) => {};

	const presenceApi = createPresenceApi({
		getMyUserId: () => myUserId,
		getChatMode: () => s.chatMode,
		getTargetUser: () => s.targetUser,
		getGroupId: () => s.groupId,
		getUserLabels: () => s.userLabels,
		setUserLabels: (l) => {
			s.userLabels = l;
		},
		getUnreadPeers: () => s.unreadPeers,
		setUnreadPeers: (u) => {
			s.unreadPeers = u;
		},
		getUnreadGroups: () => s.unreadGroups,
		setUnreadGroups: (u) => {
			s.unreadGroups = u;
		},
		getLastPreviews: () => s.lastPreviews,
		setLastPreviews: (p) => {
			s.lastPreviews = p;
		},
		getOnlineUsers: () => s.onlineUsers,
		setOnlineUsers: (u) => {
			s.onlineUsers = u;
		},
		getFriends: () => s.friends,
		setFriends: (f) => {
			s.friends = f;
		},
		getGroupMembers: () => s.groupMembers,
		setGroupMembers: (m) => {
			s.groupMembers = m;
		},
		getBalance: () => s.balance,
		setBalance: (b) => {
			s.balance = b;
		},
		refreshGroupMembers: (g) => refreshGroupMembersRef(g),
		onBalanceChange: opts.onBalanceChange
	});

	const {
		rememberUsers,
		updatePreview,
		clearUnread,
		hasUnread,
		clearGroupUnread,
		hasGroupUnread,
		ensurePeerListed,
		applyPresence,
		applyGroupMembers,
		refreshOnlineUsers,
		displayName,
		refreshBalance,
		markUnread,
		markGroupUnread
	} = presenceApi;

	displayNameRef = displayName;

	function isWsOpen(): boolean {
		return isWsOpenRef();
	}

	const typingApi = createTypingApi({
		getMyUserId: () => myUserId,
		getChatMode: () => s.chatMode,
		setChatMode: (m) => {
			s.chatMode = m;
		},
		getTargetUser: () => s.targetUser,
		setTargetUser: (v) => {
			s.targetUser = v;
		},
		getGroupId: () => s.groupId,
		setGroupId: (v) => {
			s.groupId = v;
		},
		getJoinedGroups: () => s.joinedGroups,
		getTypingUsers: () => s.typingUsers,
		setTypingUsers: (u) => {
			s.typingUsers = u;
		},
		getTypingHint: () => s.typingHint,
		setTypingHint: (h) => {
			s.typingHint = h;
		},
		getUserLabel: (uid) => s.userLabels[uid],
		rememberUsers,
		isWsOpen,
		wsSendJSON: (payload) => wsSendJSONRef(payload),
		onTypingHintChange: opts.onTypingHintChange
	});

	const {
		removeRemoteTyper,
		applyTypingEvent,
		clearTypingForConversation,
		typingLabel,
		notifyTyping,
		notifyTypingStop
	} = typingApi;

	const historyApi = createHistoryApi({
		getMyUserId: () => myUserId,
		getChatMode: () => s.chatMode,
		getTargetUser: () => s.targetUser,
		getGroupId: () => s.groupId,
		getMessages: () => s.messages,
		setMessages: (m) => {
			s.messages = m;
		},
		getBlockedIds: () => s.blacklist.map((b) => b.user_id),
		getHistoryEpoch: () => s.historyEpoch,
		bumpHistoryEpoch: () => {
			s.historyEpoch += 1;
			return s.historyEpoch;
		},
		getLoadedKey: () => s.loadedKey,
		setLoadedKey: (k) => {
			s.loadedKey = k;
		},
		setHistoryLoading: (v) => {
			s.historyLoading = v;
		},
		setHistoryLoadingOlder: (v) => {
			s.historyLoadingOlder = v;
		},
		getHistoryHasMore: () => s.historyHasMore,
		setHistoryHasMore: (v) => {
			s.historyHasMore = v;
		},
		ensureCryptoKey,
		updatePreview,
		hasMessageKey
	});

	const {
		loadPrivateHistory,
		loadGroupHistory,
		loadOlderHistory: loadOlderHistoryInner,
		clearLocalHistory,
		reloadActiveHistory
	} = historyApi;

	async function loadOlderHistory(): Promise<number> {
		if (s.historyLoadingOlder || s.historyLoading || !s.historyHasMore) return 0;
		return loadOlderHistoryInner();
	}

	const friendsApi = createFriendsApi({
		getMyUserId: () => myUserId,
		getFriends: () => s.friends,
		setFriends: (f) => {
			s.friends = f;
		},
		getIncoming: () => s.incomingRequests,
		setIncoming: (r) => {
			s.incomingRequests = r;
		},
		getBlacklist: () => s.blacklist,
		setBlacklist: (b) => {
			s.blacklist = b;
		},
		getChatMode: () => s.chatMode,
		getTargetUser: () => s.targetUser,
		setTargetUser: (v) => {
			s.targetUser = v;
		},
		setMessages: (m) => {
			s.messages = (m as ChatMessage[]) ?? [];
		},
		setLoadedKey: (k) => {
			s.loadedKey = k;
		},
		setGroupAnnouncements: (a) => {
			s.groupAnnouncements = a;
		},
		getLastPreviews: () => s.lastPreviews,
		setLastPreviews: (p) => {
			s.lastPreviews = p;
		},
		getUnreadPeers: () => s.unreadPeers,
		setUnreadPeers: (u) => {
			s.unreadPeers = u;
		},
		rememberUsers,
		filterGroupMessagesFromUser: (uid) => {
			s.messages = s.messages.filter((m) => m.from !== uid);
		}
	});

	const {
		refreshFriends,
		refreshIncomingRequests,
		inviteFriend: inviteFriendInner,
		acceptFriendRequest,
		rejectFriendRequest,
		removeFriend,
		refreshBlacklist,
		blockUser,
		unblockUser,
		isUserBlocked
	} = friendsApi;

	async function inviteFriend(username: string) {
		const name = username.trim();
		if (!name) throw new Error('Enter a username');
		return inviteFriendInner(name);
	}

	const messagingApi = createMessagingApi({
		getMyUserId: () => myUserId,
		getChatMode: () => s.chatMode,
		getTargetUser: () => s.targetUser,
		getGroupId: () => s.groupId,
		getMessages: () => s.messages,
		setMessages: (m) => {
			s.messages = m;
		},
		getInputText: () => s.inputText,
		setInputText: (v) => {
			s.inputText = v;
		},
		getReplyTarget: () => s.replyTarget,
		setReplyTargetState: (t) => {
			s.replyTarget = t;
		},
		ensureCryptoKey,
		updatePreview,
		isUserBlocked,
		removeRemoteTyper,
		ensurePeerListed,
		clearUnread,
		markUnread,
		markGroupUnread,
		notifyTypingStop,
		refreshBalance,
		wsSendReliable: (payload, sendOpts) => wsSendReliableRef(payload, sendOpts)
	});

	const {
		applyRecall,
		applyEditEvent,
		handleIncomingChat,
		sendMessage,
		resendMessage,
		flushPendingSends,
		recallMessage,
		editMessage,
		sendVoiceMessage,
		sendRedPacket,
		setReplyTarget,
		clearReplyTarget
	} = messagingApi;

	const groupsApi = createGroupsApi({
		getMyUserId: () => myUserId,
		getGroupId: () => s.groupId,
		setGroupId: (v) => {
			s.groupId = v;
		},
		getTargetUser: () => s.targetUser,
		getJoinedGroups: () => s.joinedGroups,
		setJoinedGroups: (g) => {
			s.joinedGroups = g;
		},
		getGroupMeta: () => s.groupMeta,
		setGroupMeta: (m) => {
			s.groupMeta = m;
		},
		getGroupMembers: () => s.groupMembers,
		setGroupMembers: (m) => {
			s.groupMembers = m;
		},
		getGroupAnnouncements: () => s.groupAnnouncements,
		setGroupAnnouncements: (a) => {
			s.groupAnnouncements = a;
		},
		getSelectMode: () => s.selectMode,
		setSelectMode: (v) => {
			s.selectMode = v;
		},
		getSelectedMsgIds: () => s.selectedMsgIds,
		setSelectedMsgIds: (ids) => {
			s.selectedMsgIds = ids;
		},
		getMessages: () => s.messages,
		setMessages: (m) => {
			s.messages = m;
		},
		getChatMode: () => s.chatMode,
		displayName: (uid) => displayNameRef(uid),
		wsSendJSON: (payload) => wsSendJSONRef(payload),
		isWsOpen
	});

	const {
		refreshMyGroups,
		refreshGroupMembers: refreshGroupMembersInner,
		refreshAnnouncements,
		groupDisplayName,
		isGroupOwner,
		isGroupManager,
		createGroup: createGroupInner,
		uploadGroupAvatar,
		renameGroup,
		setMemberRole,
		dissolveGroup: dissolveGroupInner,
		enterSelectMode,
		exitSelectMode,
		toggleSelectMessage,
		setMessagesAsAnnouncement,
		removeAnnouncement,
		isAnnouncement
	} = groupsApi;

	groupDisplayNameRef = groupDisplayName;

	async function refreshGroupMembers(g?: string) {
		await refreshGroupMembersInner(g);
		const list = s.groupMembers;
		if (list.length) {
			rememberUsers(list.map((m) => ({ user_id: m.user_id, username: m.username })));
		}
	}

	refreshGroupMembersRef = refreshGroupMembers;

	const meetingsApi = createMeetingsApi({
		getMyUserId: () => myUserId,
		getActiveMeetings: () => s.activeMeetings,
		setActiveMeetings: (m) => {
			s.activeMeetings = m;
		},
		groupDisplayName: (gid) => groupDisplayNameRef(gid)
	});

	const { applyMeetingEvent, refreshGroupMeeting, setActiveMeeting } = meetingsApi;

	const { handleWsMessage } = createWsDispatcher({
		getMyUserId: () => myUserId,
		getChatMode: () => s.chatMode,
		getTargetUser: () => s.targetUser,
		setTargetUser: (v) => {
			s.targetUser = v;
		},
		getGroupId: () => s.groupId,
		setGroupId: (v) => {
			s.groupId = v;
		},
		getMessages: () => s.messages,
		setMessages: (m) => {
			s.messages = m;
		},
		getLastPreviews: () => s.lastPreviews,
		setLastPreviews: (p) => {
			s.lastPreviews = p;
		},
		getUnreadPeers: () => s.unreadPeers,
		setUnreadPeers: (u) => {
			s.unreadPeers = u;
		},
		getJoinedGroups: () => s.joinedGroups,
		setJoinedGroups: (g) => {
			s.joinedGroups = g;
		},
		getGroupMeta: () => s.groupMeta,
		setGroupMeta: (m) => {
			s.groupMeta = m;
		},
		setGroupMembers: (m) => {
			s.groupMembers = m;
		},
		getGroupAnnouncements: () => s.groupAnnouncements,
		setGroupAnnouncements: (a) => {
			s.groupAnnouncements = a;
		},
		setLoadedKey: (k) => {
			s.loadedKey = k;
		},
		applyRecall,
		applyEditEvent,
		applyMeetingEvent,
		applyPresence,
		applyGroupMembers,
		applyTypingEvent,
		handleIncomingChat,
		refreshIncomingRequests,
		refreshFriends,
		refreshBlacklist,
		refreshOnlineUsers,
		refreshAnnouncements,
		refreshBalance,
		onCallEvent: opts.onCallEvent,
		onMeetingEvent: opts.onMeetingEvent,
		onRedPacketClaimed: opts.onRedPacketClaimed
	});

	const wsSession = createWsSession({
		getToken: () =>
			(typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null) ||
			opts.token,
		onUnauthorized: opts.onUnauthorized,
		onStatus: (status, attempt) => {
			s.connectionStatus = status;
			if (typeof attempt === 'number') s.reconnectAttempt = attempt;
			if (status === 'disconnected' || status === 'reconnecting') {
				s.onlineUsers = s.onlineUsers.filter((u) => u.user_id !== myUserId);
			}
		},
		onMessage: handleWsMessage,
		onOpen: (socket, gen) => onSocketReady(socket, gen),
		ensureCryptoKey
	});

	isWsOpenRef = () => {
		const sock = wsSession.getSocket();
		return !!sock && sock.readyState === WebSocket.OPEN;
	};
	wsSendJSONRef = (payload, socket?) => wsSession.wsSendJSON(payload, socket);
	wsSendReliableRef = (payload, sendOpts) => wsSession.wsSendReliable(payload, sendOpts);

	const { connect, disconnect, reconnectNow } = wsSession;

	return {
		// presence
		rememberUsers,
		updatePreview,
		clearUnread,
		hasUnread,
		clearGroupUnread,
		hasGroupUnread,
		ensurePeerListed,
		refreshOnlineUsers,
		displayName,
		refreshBalance,
		// typing
		clearTypingForConversation,
		typingLabel,
		notifyTyping,
		notifyTypingStop,
		// history
		loadPrivateHistory,
		loadGroupHistory,
		loadOlderHistory,
		clearLocalHistory,
		reloadActiveHistory,
		// friends
		refreshFriends,
		refreshIncomingRequests,
		inviteFriend,
		acceptFriendRequest,
		rejectFriendRequest,
		removeFriend,
		refreshBlacklist,
		blockUser,
		unblockUser,
		// messaging
		sendMessage,
		resendMessage,
		flushPendingSends,
		recallMessage,
		editMessage,
		sendVoiceMessage,
		sendRedPacket,
		setReplyTarget,
		clearReplyTarget,
		// groups
		refreshMyGroups,
		refreshGroupMembers,
		refreshAnnouncements,
		groupDisplayName,
		isGroupOwner,
		isGroupManager,
		createGroupInner,
		uploadGroupAvatar,
		renameGroup,
		setMemberRole,
		dissolveGroupInner,
		enterSelectMode,
		exitSelectMode,
		toggleSelectMessage,
		setMessagesAsAnnouncement,
		removeAnnouncement,
		isAnnouncement,
		// meetings
		refreshGroupMeeting,
		setActiveMeeting,
		// ws
		isWsOpen,
		wsSession,
		connect,
		disconnect,
		reconnectNow
	};
}

export type WiredChatController = ReturnType<typeof wireChatController>;
