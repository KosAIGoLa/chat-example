/**
 * Reactive chat controller state ($state) — owned by createChatController.
 * Called from Svelte component context (same as prior inline $state).
 */

import type {
	ActiveGroupMeeting,
	BlacklistUser,
	ChatMessage,
	ChatMode,
	ConnectionStatus,
	FriendRequest,
	FriendUser,
	GroupAnnouncement,
	GroupInfo,
	GroupMember,
	OnlineUser,
	ReplyTarget,
	TypingUser
} from '../types';
import { loadJoinedGroups } from './joined-groups';

export function createChatState(myUserId: string) {
	let messages = $state<ChatMessage[]>([]);
	let inputText = $state('');
	let targetUser = $state('');
	let groupId = $state('');
	let chatMode = $state<ChatMode>('private');
	let joinedGroups = $state<string[]>(loadJoinedGroups());
	/** Durable group metadata (name, role) keyed by id. */
	let groupMeta = $state<Record<string, GroupInfo>>({});
	/** Global online users — used for private DM list only (never includes self). */
	let onlineUsers = $state<OnlineUser[]>([]);
	/** Accepted friends (primary private chat list). */
	let friends = $state<FriendUser[]>([]);
	/** Incoming friend invites waiting for my accept/reject. */
	let incomingRequests = $state<FriendRequest[]>([]);
	/** Users I blocked. */
	let blacklist = $state<BlacklistUser[]>([]);
	/** Full durable roster of the selected group (role + online; includes self). */
	let groupMembers = $state<GroupMember[]>([]);
	/** user_id → username cache for titles / labels. */
	let userLabels = $state<Record<string, string>>({});
	/** user_ids with unread private messages (blink in list). */
	let unreadPeers = $state<Record<string, boolean>>({});
	/** group_ids with unread group messages. */
	let unreadGroups = $state<Record<string, boolean>>({});
	/** Last message preview per conversation key: private:uid | group:gid */
	let lastPreviews = $state<Record<string, { text: string; ts: number }>>({});
	/** Virtual wallet balance (coins). */
	let balance = $state(0);
	/**
	 * Open group conferences (meeting mode), keyed by group_id.
	 * Distinct from private 1:1 calls — members join freely.
	 */
	let activeMeetings = $state<Record<string, ActiveGroupMeeting>>({});
	/** Users currently typing in the active conversation (private peer or group). */
	let typingUsers = $state<TypingUser[]>([]);
	/** Preformatted hint for UI (kept in sync for reliable reactivity). */
	let typingHint = $state('');
	/** Group chat: reply-to member (optional quote). */
	let replyTarget = $state<ReplyTarget | null>(null);
	/** Current group's pinned announcements. */
	let groupAnnouncements = $state<GroupAnnouncement[]>([]);
	/** Multi-select message ids for bulk pin as announcement. */
	let selectMode = $state(false);
	let selectedMsgIds = $state<string[]>([]);
	let connectionStatus = $state<ConnectionStatus>('disconnected');
	let historyLoading = $state(false);
	/** True while fetching an older page (scroll-up). */
	let historyLoadingOlder = $state(false);
	/** Server has more older messages than currently loaded. */
	let historyHasMore = $state(true);
	/** How many reconnect attempts since last successful open. */
	let reconnectAttempt = $state(0);

	/** Bumps when conversation changes so in-flight history loads can be ignored. */
	let historyEpoch = 0;
	/** Avoid reloading the same conversation on input blur. */
	let loadedKey = '';

	return {
		get myUserId() {
			return myUserId;
		},

		get messages() {
			return messages;
		},
		set messages(v: ChatMessage[]) {
			messages = v;
		},
		get inputText() {
			return inputText;
		},
		set inputText(v: string) {
			inputText = v;
		},
		get targetUser() {
			return targetUser;
		},
		set targetUser(v: string) {
			targetUser = v;
		},
		get groupId() {
			return groupId;
		},
		set groupId(v: string) {
			groupId = v;
		},
		get chatMode() {
			return chatMode;
		},
		set chatMode(v: ChatMode) {
			chatMode = v;
		},
		get joinedGroups() {
			return joinedGroups;
		},
		set joinedGroups(v: string[]) {
			joinedGroups = v;
		},
		get groupMeta() {
			return groupMeta;
		},
		set groupMeta(v: Record<string, GroupInfo>) {
			groupMeta = v;
		},
		get onlineUsers() {
			return onlineUsers;
		},
		set onlineUsers(v: OnlineUser[]) {
			onlineUsers = v;
		},
		get friends() {
			return friends;
		},
		set friends(v: FriendUser[]) {
			friends = v;
		},
		get incomingRequests() {
			return incomingRequests;
		},
		set incomingRequests(v: FriendRequest[]) {
			incomingRequests = v;
		},
		get blacklist() {
			return blacklist;
		},
		set blacklist(v: BlacklistUser[]) {
			blacklist = v;
		},
		get groupMembers() {
			return groupMembers;
		},
		set groupMembers(v: GroupMember[]) {
			groupMembers = v;
		},
		get userLabels() {
			return userLabels;
		},
		set userLabels(v: Record<string, string>) {
			userLabels = v;
		},
		get unreadPeers() {
			return unreadPeers;
		},
		set unreadPeers(v: Record<string, boolean>) {
			unreadPeers = v;
		},
		get unreadGroups() {
			return unreadGroups;
		},
		set unreadGroups(v: Record<string, boolean>) {
			unreadGroups = v;
		},
		get lastPreviews() {
			return lastPreviews;
		},
		set lastPreviews(v: Record<string, { text: string; ts: number }>) {
			lastPreviews = v;
		},
		get balance() {
			return balance;
		},
		set balance(v: number) {
			balance = v;
		},
		get activeMeetings() {
			return activeMeetings;
		},
		set activeMeetings(v: Record<string, ActiveGroupMeeting>) {
			activeMeetings = v;
		},
		get typingUsers() {
			return typingUsers;
		},
		set typingUsers(v: TypingUser[]) {
			typingUsers = v;
		},
		get typingHint() {
			return typingHint;
		},
		set typingHint(v: string) {
			typingHint = v;
		},
		get replyTarget() {
			return replyTarget;
		},
		set replyTarget(v: ReplyTarget | null) {
			replyTarget = v;
		},
		get groupAnnouncements() {
			return groupAnnouncements;
		},
		set groupAnnouncements(v: GroupAnnouncement[]) {
			groupAnnouncements = v;
		},
		get selectMode() {
			return selectMode;
		},
		set selectMode(v: boolean) {
			selectMode = v;
		},
		get selectedMsgIds() {
			return selectedMsgIds;
		},
		set selectedMsgIds(v: string[]) {
			selectedMsgIds = v;
		},
		get connectionStatus() {
			return connectionStatus;
		},
		set connectionStatus(v: ConnectionStatus) {
			connectionStatus = v;
		},
		get historyLoading() {
			return historyLoading;
		},
		set historyLoading(v: boolean) {
			historyLoading = v;
		},
		get historyLoadingOlder() {
			return historyLoadingOlder;
		},
		set historyLoadingOlder(v: boolean) {
			historyLoadingOlder = v;
		},
		get historyHasMore() {
			return historyHasMore;
		},
		set historyHasMore(v: boolean) {
			historyHasMore = v;
		},
		get reconnectAttempt() {
			return reconnectAttempt;
		},
		set reconnectAttempt(v: number) {
			reconnectAttempt = v;
		},

		get historyEpoch() {
			return historyEpoch;
		},
		set historyEpoch(v: number) {
			historyEpoch = v;
		},
		get loadedKey() {
			return loadedKey;
		},
		set loadedKey(v: string) {
			loadedKey = v;
		}
	};
}

export type ChatState = ReturnType<typeof createChatState>;
