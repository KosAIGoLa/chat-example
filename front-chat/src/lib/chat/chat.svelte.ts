/**
 * Chat controller orchestrator — reactive $state + domain wiring.
 * Domain logic lives under ./controller/.
 */
import { chatService } from '$lib/api';
import { hasMessageKey, importMessageKeyFromWrapped } from './crypto';
import type {
	CallEvent,
	MeetingEvent,
	RedPacketClaimedEvent
} from './types';
import { createChatState } from './controller/create-chat-state.svelte';
import { wireChatController } from './controller/wire-controller';
import { createConversationNav } from './controller/conversation-nav';

export function createChatController(opts: {
	token: string;
	userId: string;
	onUnauthorized?: () => void;
	/** LiveKit private call signaling events from the chat WebSocket. */
	onCallEvent?: (ev: CallEvent) => void;
	/** Group conference lifecycle (meeting mode — not private ring). */
	onMeetingEvent?: (ev: MeetingEvent) => void;
	/** Balance changed (red packet send/claim). */
	onBalanceChange?: (balance: number) => void;
	/** Red packet claimed by anyone (refresh cards). */
	onRedPacketClaimed?: (ev: RedPacketClaimedEvent) => void;
	/** Typing indicator label changed (e.g. "Alice 正在输入…"). */
	onTypingHintChange?: (label: string) => void;
}) {
	const myUserId = opts.userId;
	const state = createChatState(myUserId);

	async function ensureCryptoKey(): Promise<void> {
		if (hasMessageKey()) return;
		const res = await chatService.getCryptoKey();
		const token =
			(typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null) ||
			opts.token ||
			'';
		if (!token) {
			throw new Error('missing auth token for crypto key unwrap');
		}
		if (!res?.w) {
			throw new Error('crypto key response missing wrapped blob');
		}
		await importMessageKeyFromWrapped(res.w, token);
	}

	// Forward ref: nav.onSocketReady is assigned after wire (needs wired APIs).
	let onSocketReadyRef = async (_socket: WebSocket, _gen: number) => {};

	const wired = wireChatController({
		state,
		opts,
		ensureCryptoKey,
		onSocketReady: (socket, gen) => onSocketReadyRef(socket, gen)
	});

	const nav = createConversationNav({
		state,
		myUserId,
		wired
	});
	onSocketReadyRef = nav.onSocketReady;

	return {
		get replyTarget() {
			return state.replyTarget;
		},
		setReplyTarget: wired.setReplyTarget,
		clearReplyTarget: wired.clearReplyTarget,
		get groupAnnouncements() {
			return state.groupAnnouncements;
		},
		get selectMode() {
			return state.selectMode;
		},
		get selectedMsgIds() {
			return state.selectedMsgIds;
		},
		refreshAnnouncements: wired.refreshAnnouncements,
		enterSelectMode: wired.enterSelectMode,
		exitSelectMode: wired.exitSelectMode,
		toggleSelectMessage: wired.toggleSelectMessage,
		setMessagesAsAnnouncement: wired.setMessagesAsAnnouncement,
		removeAnnouncement: wired.removeAnnouncement,
		isAnnouncement: wired.isAnnouncement,
		get messages() {
			return state.messages;
		},
		get inputText() {
			return state.inputText;
		},
		set inputText(v: string) {
			state.inputText = v;
		},
		get targetUser() {
			return state.targetUser;
		},
		set targetUser(v: string) {
			state.targetUser = v;
		},
		get groupId() {
			return state.groupId;
		},
		set groupId(v: string) {
			state.groupId = v;
		},
		get chatMode() {
			return state.chatMode;
		},
		get activeMeetings() {
			return state.activeMeetings;
		},
		get joinedGroups() {
			return state.joinedGroups;
		},
		get groupMeta() {
			return state.groupMeta;
		},
		get onlineUsers() {
			return state.onlineUsers;
		},
		get friends() {
			return state.friends;
		},
		get incomingRequests() {
			return state.incomingRequests;
		},
		get blacklist() {
			return state.blacklist;
		},
		get groupMembers() {
			return state.groupMembers;
		},
		get unreadPeers() {
			return state.unreadPeers;
		},
		get unreadGroups() {
			return state.unreadGroups;
		},
		get lastPreviews() {
			return state.lastPreviews;
		},
		get balance() {
			return state.balance;
		},
		get typingUsers() {
			return state.typingUsers;
		},
		get typingHint() {
			return state.typingHint;
		},
		get connectionStatus() {
			return state.connectionStatus;
		},
		get reconnectAttempt() {
			return state.reconnectAttempt;
		},
		get myUserId() {
			return myUserId;
		},
		get historyLoading() {
			return state.historyLoading;
		},
		get historyLoadingOlder() {
			return state.historyLoadingOlder;
		},
		get historyHasMore() {
			return state.historyHasMore;
		},
		displayName: wired.displayName,
		hasUnread: wired.hasUnread,
		hasGroupUnread: wired.hasGroupUnread,
		typingLabel: wired.typingLabel,
		connect: wired.connect,
		disconnect: wired.disconnect,
		reconnectNow: wired.reconnectNow,
		refreshOnlineUsers: wired.refreshOnlineUsers,
		refreshFriends: wired.refreshFriends,
		refreshIncomingRequests: wired.refreshIncomingRequests,
		refreshGroupMembers: wired.refreshGroupMembers,
		inviteFriend: wired.inviteFriend,
		acceptFriendRequest: wired.acceptFriendRequest,
		rejectFriendRequest: wired.rejectFriendRequest,
		removeFriend: wired.removeFriend,
		refreshBlacklist: wired.refreshBlacklist,
		blockUser: wired.blockUser,
		unblockUser: wired.unblockUser,
		sendMessage: wired.sendMessage,
		sendVoiceMessage: wired.sendVoiceMessage,
		sendRedPacket: wired.sendRedPacket,
		resendMessage: wired.resendMessage,
		refreshBalance: wired.refreshBalance,
		notifyTyping: wired.notifyTyping,
		notifyTypingStop: wired.notifyTypingStop,
		recallMessage: wired.recallMessage,
		editMessage: wired.editMessage,
		joinGroup: nav.joinGroup,
		leaveGroup: nav.leaveGroup,
		createGroup: nav.createGroup,
		dissolveGroup: nav.dissolveGroup,
		refreshMyGroups: wired.refreshMyGroups,
		uploadGroupAvatar: wired.uploadGroupAvatar,
		renameGroup: wired.renameGroup,
		setMemberRole: wired.setMemberRole,
		isGroupOwner: wired.isGroupOwner,
		isGroupManager: wired.isGroupManager,
		groupDisplayName: wired.groupDisplayName,
		selectPrivateUser: nav.selectPrivateUser,
		selectGroup: nav.selectGroup,
		setChatMode: nav.setChatMode,
		refreshGroupMeeting: wired.refreshGroupMeeting,
		setActiveMeeting: wired.setActiveMeeting,
		loadPrivateHistory: wired.loadPrivateHistory,
		loadGroupHistory: wired.loadGroupHistory,
		loadOlderHistory: wired.loadOlderHistory,
		clearLocalHistory: wired.clearLocalHistory
	};
}

export type ChatController = ReturnType<typeof createChatController>;
