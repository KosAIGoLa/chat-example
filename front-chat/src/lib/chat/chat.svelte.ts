import {
	buildWsUrl,
	chatService,
	friendService,
	groupService,
	mediaService,
	redPacketService,
	type CreateRedPacketBody
} from '$lib/api';
import {
	encryptContent,
	hasMessageKey,
	importMessageKeyFromWrapped,
	isEncryptedContent,
	sealWSFrame,
	tryDecryptContent,
	tryOpenWSFrame
} from './crypto';
import type {
	ChatMessage,
	ChatMode,
	ConnectionStatus,
	BlacklistUser,
	CallEvent,
	FriendEvent,
	FriendRequest,
	FriendUser,
	GroupDissolvedEvent,
	GroupInfo,
	GroupMembersEvent,
	OnlineUser,
	PingMessage,
	PongMessage,
	PresenceEvent,
	RecallEvent,
	RedPacketClaimedEvent,
	OfflineSyncEvent,
	TypingEvent,
	TypingUser
} from './types';
import { messagePreview } from './utils';
import { toastError, toastInfo } from '$lib/ui/notify.svelte';
import {
	activeConvKey,
	clearTypingHint,
	setTypingHint,
	typingUI
} from './typing-ui.svelte';
import {
	convKeyGroup,
	convKeyPrivate,
	loadConvCache,
	maxSeqOf,
	mergeById,
	saveConvCache,
	sortMessagesBySeq
} from './message-cache';

const JOINED_GROUPS_KEY = 'joined_groups';

function loadJoinedGroups(): string[] {
	if (typeof window === 'undefined') return [];
	try {
		const raw = localStorage.getItem(JOINED_GROUPS_KEY);
		if (!raw) return [];
		const parsed = JSON.parse(raw);
		return Array.isArray(parsed) ? parsed.filter((g) => typeof g === 'string') : [];
	} catch {
		return [];
	}
}

function saveJoinedGroups(groups: string[]) {
	if (typeof window === 'undefined') return;
	localStorage.setItem(JOINED_GROUPS_KEY, JSON.stringify(groups));
}

function isChatContent(msg: ChatMessage): boolean {
	if (msg.type !== 'private' && msg.type !== 'group') return false;
	// Recalled messages keep their slot in history with empty body.
	if (msg.recalled) return true;
	if (msg.content_type === 'voice') return !!msg.media_url;
	if (msg.content_type === 'red_packet') return !!msg.red_packet_id || !!msg.content;
	// System notices (join/leave) and normal text both need non-empty content.
	return !!msg.content;
}

async function decryptMessage(msg: ChatMessage): Promise<ChatMessage> {
	if (!msg.content || (!msg.encrypted && !isEncryptedContent(msg.content))) {
		return msg;
	}
	const plain = await tryDecryptContent(msg.content);
	return { ...msg, content: plain, encrypted: false };
}

async function decryptMessages(list: ChatMessage[]): Promise<ChatMessage[]> {
	return Promise.all(list.map((m) => decryptMessage(m)));
}

function belongsToConversation(
	msg: ChatMessage,
	mode: ChatMode,
	myUserId: string,
	peerId: string,
	activeGroupId: string
): boolean {
	if (mode === 'private') {
		if (msg.type !== 'private' || !peerId) return false;
		return (
			(msg.from === myUserId && msg.to === peerId) ||
			(msg.from === peerId && msg.to === myUserId)
		);
	}
	if (msg.type !== 'group' || !activeGroupId) return false;
	const gid = msg.group_id || msg.to;
	return gid === activeGroupId;
}

function messageKey(msg: ChatMessage): string {
	if (msg.id) return `id:${msg.id}`;
	return `${msg.type}|${msg.from}|${msg.to}|${msg.group_id ?? ''}|${msg.content_type ?? 'text'}|${msg.media_url ?? ''}|${msg.content}|${msg.timestamp ?? 0}`;
}

function newMsgId(): string {
	if (typeof crypto !== 'undefined' && crypto.randomUUID) {
		return crypto.randomUUID().replace(/-/g, '');
	}
	return `${Date.now().toString(16)}${Math.random().toString(16).slice(2, 14)}`.padEnd(32, '0').slice(0, 32);
}

/** Reconnect backoff: 1s → 2s → 4s … capped at 30s. */
const RECONNECT_BASE_MS = 1000;
const RECONNECT_MAX_MS = 30_000;
const RECONNECT_MAX_ATTEMPTS = 50;

/**
 * Application-level heartbeat (encrypted JSON frames).
 * Server read deadline is 60s; ping well under that and fail if no pong.
 */
const HEARTBEAT_INTERVAL_MS = 20_000;
const HEARTBEAT_TIMEOUT_MS = 10_000;

export function createChatController(opts: {
	token: string;
	userId: string;
	onUnauthorized?: () => void;
	/** LiveKit call signaling events from the chat WebSocket. */
	onCallEvent?: (ev: CallEvent) => void;
	/** Balance changed (red packet send/claim). */
	onBalanceChange?: (balance: number) => void;
	/** Red packet claimed by anyone (refresh cards). */
	onRedPacketClaimed?: (ev: RedPacketClaimedEvent) => void;
	/** Typing indicator label changed (e.g. "Alice 正在输入…"). */
	onTypingHintChange?: (label: string) => void;
}) {
	let ws: WebSocket | null = null;
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
	/** Members of the currently selected group (never includes self). */
	let groupMembers = $state<OnlineUser[]>([]);
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
	/** Users currently typing in the active conversation (private peer or group). */
	let typingUsers = $state<TypingUser[]>([]);
	/** Preformatted hint for UI (kept in sync for reliable reactivity). */
	let typingHint = $state('');
	let connectionStatus = $state<ConnectionStatus>('disconnected');
	let historyLoading = $state(false);
	/** How many reconnect attempts since last successful open. */
	let reconnectAttempt = $state(0);

	const myUserId = opts.userId;
	/** Bumps when conversation changes so in-flight history loads can be ignored. */
	let historyEpoch = 0;
	/** Avoid reloading the same conversation on input blur. */
	let loadedKey = '';

	/** When true, do not auto-reconnect (user logout / intentional close). */
	let intentionalClose = false;
	/** Incremented to cancel pending reconnect timers. */
	let reconnectGen = 0;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	/** Generation for the active socket so stale handlers ignore events. */
	let socketGen = 0;
	/** Browser online/offline + visibility listeners attached once. */
	let networkHooksAttached = false;
	/**
	 * Serialize connectAsync: ensureCryptoKey() yields, and without a lock two
	 * concurrent connect() calls both open sockets and thrash each other.
	 */
	let connectInFlight: Promise<void> | null = null;
	/** Heartbeat interval + outstanding pong timer for the active socket. */
	let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
	let heartbeatTimeout: ReturnType<typeof setTimeout> | null = null;
	/** Last successful pong time (ms); useful for debugging / RTT. */
	let lastPongAt = 0;
	/**
	 * Outbound typing state machine:
	 *   idle → (keystroke) → active (send start) → (idle timeout) → send stop → idle
	 * Remote side:
	 *   start → show + TTL; stop → clear immediately; TTL expire → clear
	 */
	let typingActive = false;
	let lastTypingPingAt = 0;
	let typingIdleTimer: ReturnType<typeof setTimeout> | null = null;
	let typingPruneTimer: ReturnType<typeof setInterval> | null = null;
	/** Last outbound session so idle-stop always uses the same peer/group (群主关键). */
	let lastTypingSession: { mode: ChatMode; peer: string; group: string } | null = null;
	/** How long a remote typing flag stays without refresh (ms). Hard stop even if stop lost. */
	const TYPING_TTL_MS = 3000;
	/** Re-send "start" at most this often while continuously typing. */
	const TYPING_PING_MS = 1500;
	/** After last keystroke, send "stop". */
	const TYPING_IDLE_MS = 1500;

	function withoutSelf(list: OnlineUser[]): OnlineUser[] {
		return list.filter((u) => u.user_id && u.user_id !== myUserId);
	}

	function rememberUsers(users: OnlineUser[]) {
		if (!users.length) return;
		const next = { ...userLabels };
		for (const u of users) {
			if (u.user_id && u.username) next[u.user_id] = u.username;
		}
		userLabels = next;
	}

	function normalizeOnlineList(raw: unknown): OnlineUser[] {
		if (!Array.isArray(raw)) return [];
		const out: OnlineUser[] = [];
		for (const item of raw) {
			if (typeof item === 'string') {
				// Backward-compatible: old API returned bare ids.
				out.push({ user_id: item, username: item });
				continue;
			}
			if (item && typeof item === 'object') {
				const o = item as Record<string, unknown>;
				const uid = String(o.user_id ?? o.id ?? '');
				if (!uid) continue;
				const name = String(o.username ?? o.name ?? uid);
				out.push({ user_id: uid, username: name || uid });
			}
		}
		return withoutSelf(out);
	}

	function conversationKey(msg: ChatMessage): string | null {
		if (msg.type === 'private') {
			const peer = msg.from === myUserId ? msg.to : msg.from;
			return peer ? `private:${peer}` : null;
		}
		if (msg.type === 'group') {
			const gid = msg.group_id || msg.to;
			return gid ? `group:${gid}` : null;
		}
		return null;
	}

	function updatePreview(msg: ChatMessage) {
		const key = conversationKey(msg);
		if (!key) return;
		const ts = msg.timestamp ?? 0;
		const prev = lastPreviews[key];
		if (prev && prev.ts > ts) return;
		lastPreviews = {
			...lastPreviews,
			[key]: { text: messagePreview(msg), ts }
		};
	}

	function markUnread(peerId: string) {
		const id = String(peerId ?? '');
		if (!id || id === myUserId) return;
		// Already viewing this private chat — stay normal, no blink.
		if (chatMode === 'private' && String(targetUser) === id) return;
		if (unreadPeers[id]) return;
		unreadPeers = { ...unreadPeers, [id]: true };
	}

	/** Clicking a blinking peer opens the chat and turns the indicator normal. */
	function clearUnread(peerId: string) {
		const id = String(peerId ?? '');
		if (!id) return;
		if (!unreadPeers[id]) return;
		const next = { ...unreadPeers };
		delete next[id];
		unreadPeers = next;
	}

	function hasUnread(peerId: string): boolean {
		return !!unreadPeers[String(peerId ?? '')];
	}

	function markGroupUnread(gid: string) {
		const id = String(gid ?? '');
		if (!id) return;
		if (chatMode === 'group' && String(groupId) === id) return;
		if (unreadGroups[id]) return;
		unreadGroups = { ...unreadGroups, [id]: true };
	}

	function clearGroupUnread(gid: string) {
		const id = String(gid ?? '');
		if (!id || !unreadGroups[id]) return;
		const next = { ...unreadGroups };
		delete next[id];
		unreadGroups = next;
	}

	function hasGroupUnread(gid: string): boolean {
		return !!unreadGroups[String(gid ?? '')];
	}

	async function refreshBalance() {
		try {
			const w = await redPacketService.getWallet();
			balance = w.balance ?? 0;
			opts.onBalanceChange?.(balance);
		} catch {
			// ignore
		}
	}

	function currentConvKey(): string {
		return activeConvKey(chatMode, targetUser, groupId);
	}

	function formatTypingLabel(list: TypingUser[]): string {
		if (list.length === 0) return '';
		// Prefer list length heuristics over chatMode — safer if mode briefly desyncs.
		const names = list.map((t) => t.username || t.user_id);
		if (chatMode === 'private' || (names.length === 1 && !groupId)) {
			return `${names[0] || '对方'} 正在输入…`;
		}
		if (names.length === 1) return `${names[0]} 正在输入…`;
		if (names.length === 2) return `${names[0]}、${names[1]} 正在输入…`;
		return `${names[0]} 等 ${names.length} 人正在输入…`;
	}

	function publishTypingHint(list: TypingUser[] = typingUsers) {
		const label = list.length === 0 ? '' : formatTypingLabel(list);
		typingHint = label;
		if (label) {
			setTypingHint(label, currentConvKey());
		} else {
			// Force clear — do not leave stale "群主正在输入"
			clearTypingHint();
		}
		opts.onTypingHintChange?.(label);
	}

	function pruneTypingUsers() {
		const now = Date.now();
		const next = typingUsers.filter((t) => t.until > now);
		if (next.length !== typingUsers.length) {
			typingUsers = next;
			publishTypingHint(next);
		} else if (typingUsers.length === 0 && (typingHint || typingUI.hint)) {
			// Heal stuck UI if list empty but hint remains
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
		const before = typingUsers.length;
		const next = typingUsers.filter((t) => t.user_id !== id);
		if (next.length !== before) {
			typingUsers = next;
			publishTypingHint(next);
		}
	}

	function applyTypingEvent(ev: TypingEvent) {
		const from = String(ev.from ?? '');
		if (!from || from === myUserId) return;

		const action = String(ev.content || 'start').toLowerCase();
		const isStop = action === 'stop' || action === '0' || action === 'false';

		// STOP: always drop this user if present (no conversation filter).
		// Fixes stuck "群主正在输入" when stop arrives after a brief mode/group desync.
		if (isStop) {
			removeRemoteTyper(from);
			return;
		}

		const evGroup = String(ev.group_id ?? '').trim();
		const activeGroup = String(groupId ?? '').trim();
		const activePeer = String(targetUser ?? '').trim();

		// START: only show for the conversation currently open.
		if (evGroup) {
			if (chatMode !== 'group') return;
			if (activeGroup && activeGroup !== evGroup) return;
			if (!activeGroup && !joinedGroups.includes(evGroup)) return;
			if (!activeGroup) groupId = evGroup;
		} else {
			if (chatMode !== 'private') return;
			if (!activePeer || activePeer !== from) return;
		}

		const name = (ev.from_name || userLabels[from] || from).trim() || from;
		const until = Date.now() + TYPING_TTL_MS;
		const rest = typingUsers.filter((t) => t.user_id !== from);
		const next = [...rest, { user_id: from, username: name, until }];
		typingUsers = next;
		publishTypingHint(next);
		ensureTypingPrune();
		if (name && from) {
			rememberUsers([{ user_id: from, username: name }]);
		}
	}

	function clearTypingForConversation() {
		typingUsers = [];
		typingHint = '';
		clearTypingHint();
		opts.onTypingHintChange?.('');
	}

	/** Human-readable line for UI. */
	function typingLabel(): string {
		pruneTypingUsers();
		return typingHint || formatTypingLabel(typingUsers) || typingUI.hint;
	}

	function resolveTypingSession(session?: {
		mode?: ChatMode;
		peer?: string;
		group?: string;
	}): { mode: ChatMode; peer: string; group: string } | null {
		let mode: ChatMode = session?.mode ?? chatMode;
		const peer = (session?.peer ?? targetUser).trim();
		const group = (session?.group ?? groupId).trim();

		// Prefer group when both group mode and group id are present (群主/群员).
		if ((session?.mode === 'group' || chatMode === 'group') && group) {
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

	/**
	 * Notify peer/group that I am typing (throttled).
	 * Call from input; pass UI session so 群主 groupId is never lost on idle-stop.
	 */
	function notifyTyping(session?: { mode?: ChatMode; peer?: string; group?: string }) {
		const resolved = resolveTypingSession(session);
		if (!resolved) return;
		if (!ws || ws.readyState !== WebSocket.OPEN) return;

		const { mode, peer, group } = resolved;
		chatMode = mode;
		if (peer) targetUser = peer;
		if (group) groupId = group;

		// Remember session for idle-stop (critical for group owner stop).
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
							from: myUserId,
							to: peer,
							content: 'start',
							timestamp: Math.floor(now / 1000)
						}
					: {
							type: 'typing' as const,
							from: myUserId,
							to: group,
							group_id: group,
							content: 'start',
							timestamp: Math.floor(now / 1000)
						};
			void wsSendJSON(payload).catch((err) => {
				console.warn('[typing] start failed', err);
			});
		}

		// Reset idle → stop timer on every keystroke.
		if (typingIdleTimer != null) clearTimeout(typingIdleTimer);
		typingIdleTimer = setTimeout(() => {
			typingIdleTimer = null;
			// Use lastTypingSession snapshot, not live controller state.
			notifyTypingStop(lastTypingSession ?? resolved);
		}, TYPING_IDLE_MS);
	}

	function notifyTypingStop(session?: { mode?: ChatMode; peer?: string; group?: string } | null) {
		if (typingIdleTimer != null) {
			clearTimeout(typingIdleTimer);
			typingIdleTimer = null;
		}

		// Only send stop if we previously advertised start.
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
		if (!ws || ws.readyState !== WebSocket.OPEN) return;

		const { mode, peer, group } = resolved;
		const payload =
			mode === 'private'
				? {
						type: 'typing' as const,
						from: myUserId,
						to: peer,
						content: 'stop',
						timestamp: Math.floor(Date.now() / 1000)
					}
				: {
						type: 'typing' as const,
						from: myUserId,
						to: group,
						group_id: group,
						content: 'stop',
						timestamp: Math.floor(Date.now() / 1000)
					};
		void wsSendJSON(payload).catch((err) => {
			console.warn('[typing] stop failed', err);
		});
	}

	/** Ensure peer appears in private online list (e.g. DM from someone not yet listed). */
	function ensurePeerListed(peerId: string, username?: string) {
		if (!peerId || peerId === myUserId) return;
		const name = username?.trim() || userLabels[peerId] || peerId;
		rememberUsers([{ user_id: peerId, username: name }]);
		if (!onlineUsers.some((u) => u.user_id === peerId)) {
			onlineUsers = [...onlineUsers, { user_id: peerId, username: name }];
		}
	}

	function applyPresence(event: PresenceEvent) {
		const uid = String(event.user_id ?? '');
		if (!uid || uid === myUserId) return; // never list yourself
		if (event.online) {
			const name = (event.username && event.username.trim()) || uid;
			rememberUsers([{ user_id: uid, username: name }]);
			const idx = onlineUsers.findIndex((u) => u.user_id === uid);
			if (idx >= 0) {
				const next = [...onlineUsers];
				next[idx] = { user_id: uid, username: name };
				onlineUsers = next;
			} else {
				onlineUsers = [...onlineUsers, { user_id: uid, username: name }];
			}
		} else {
			// Always remove on offline — source of truth from server event.
			onlineUsers = onlineUsers.filter((u) => u.user_id !== uid);
			// Also drop from current group member list if present.
			groupMembers = groupMembers.filter((u) => u.user_id !== uid);
		}
	}

	function applyGroupMembers(event: GroupMembersEvent) {
		const gid = String(event.group_id ?? '');
		if (!gid || gid !== groupId) return;
		const members = normalizeOnlineList(event.members);
		groupMembers = members;
		rememberUsers(members);
	}

	async function handleIncomingChat(msg: ChatMessage) {
		const plain = await decryptMessage(msg);
		updatePreview(plain);
		// Any real chat message implies they stopped typing.
		if (plain.from && plain.from !== myUserId) {
			removeRemoteTyper(plain.from);
		}

		// Private DM from someone else while not viewing that chat → unread blink.
		if (plain.type === 'private' && plain.from && plain.to === myUserId) {
			const from = String(plain.from);
			if (from === myUserId) return;
			ensurePeerListed(from);
			const viewing = chatMode === 'private' && String(targetUser) === from;
			if (viewing) {
				// Already in this DM — show message, keep indicator normal.
				clearUnread(from);
				appendMessage(plain);
			} else {
				// Not viewing → amber blink until user clicks that peer.
				markUnread(from);
			}
			return;
		}

		// Group message while not viewing that group.
		if (plain.type === 'group' && plain.from !== myUserId) {
			const gid = plain.group_id || plain.to;
			if (gid && !(chatMode === 'group' && String(groupId) === String(gid))) {
				markGroupUnread(String(gid));
				return;
			}
		}

		if (!belongsToConversation(plain, chatMode, myUserId, targetUser, groupId)) {
			return;
		}
		appendMessage(plain);
	}

	async function ensureCryptoKey(): Promise<void> {
		if (hasMessageKey()) return;
		const res = await chatService.getCryptoKey();
		// Response is JWT-wrapped + obfuscated fields (v/a/cv/w) — never cleartext key.
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

	function clearReconnectTimer() {
		if (reconnectTimer != null) {
			clearTimeout(reconnectTimer);
			reconnectTimer = null;
		}
	}

	function clearHeartbeat() {
		if (heartbeatTimer != null) {
			clearInterval(heartbeatTimer);
			heartbeatTimer = null;
		}
		if (heartbeatTimeout != null) {
			clearTimeout(heartbeatTimeout);
			heartbeatTimeout = null;
		}
	}

	/**
	 * Start client → server application pings on an open socket.
	 * If a pong does not arrive within HEARTBEAT_TIMEOUT_MS, force-close so
	 * onclose schedules a reconnect (zombie TCP / half-open links).
	 */
	function startHeartbeat(socket: WebSocket, gen: number) {
		clearHeartbeat();
		lastPongAt = Date.now();

		const sendPing = () => {
			if (gen !== socketGen || ws !== socket || socket.readyState !== WebSocket.OPEN) {
				clearHeartbeat();
				return;
			}
			// Already waiting for a pong — do not pile up pings.
			if (heartbeatTimeout != null) return;

			const ts = Date.now();
			const payload: PingMessage = { type: 'ping', ts };
			void wsSendJSON(payload, socket).catch((err) => {
				console.warn('[ws] heartbeat ping failed', err);
			});

			heartbeatTimeout = setTimeout(() => {
				heartbeatTimeout = null;
				if (gen !== socketGen || ws !== socket) return;
				console.warn(
					`[ws] heartbeat timeout (${HEARTBEAT_TIMEOUT_MS}ms) — closing stale socket`
				);
				// Force close; onclose will reconnect (handlers still attached).
				try {
					socket.close(4000, 'heartbeat timeout');
				} catch {
					// ignore
				}
			}, HEARTBEAT_TIMEOUT_MS);
		};

		// First ping after one interval; then every interval.
		heartbeatTimer = setInterval(sendPing, HEARTBEAT_INTERVAL_MS);
	}

	function onPong(msg: PongMessage) {
		if (heartbeatTimeout != null) {
			clearTimeout(heartbeatTimeout);
			heartbeatTimeout = null;
		}
		lastPongAt = Date.now();
		if (typeof msg.ts === 'number' && msg.ts > 0) {
			const rtt = lastPongAt - msg.ts;
			if (rtt >= 0 && rtt < 60_000) {
				// Keep quiet in production; enable when diagnosing lag.
				// console.debug(`[ws] heartbeat rtt=${rtt}ms`);
			}
		}
	}

	function scheduleReconnect(reason: string) {
		if (intentionalClose) return;
		if (typeof navigator !== 'undefined' && navigator.onLine === false) {
			connectionStatus = 'disconnected';
			console.info('[ws] offline — wait for network before reconnect');
			return;
		}
		if (reconnectAttempt >= RECONNECT_MAX_ATTEMPTS) {
			connectionStatus = 'disconnected';
			console.warn('[ws] max reconnect attempts reached');
			return;
		}

		clearReconnectTimer();
		const attempt = reconnectAttempt + 1;
		reconnectAttempt = attempt;
		const delay = Math.min(
			RECONNECT_MAX_MS,
			RECONNECT_BASE_MS * Math.pow(2, Math.min(attempt - 1, 5))
		);
		// small jitter so multiple tabs don't thundering-herd
		const jitter = Math.floor(Math.random() * 300);
		connectionStatus = 'reconnecting';
		const gen = ++reconnectGen;
		console.info(`[ws] reconnect in ${delay + jitter}ms (attempt ${attempt}) — ${reason}`);

		reconnectTimer = setTimeout(() => {
			if (gen !== reconnectGen || intentionalClose) return;
			connect({ isReconnect: true });
		}, delay + jitter);
	}

	/** Send application JSON over WS, sealed with AES-GCM frame envelope. */
	async function wsSendJSON(payload: unknown, socket: WebSocket = ws!): Promise<void> {
		if (!socket || socket.readyState !== WebSocket.OPEN) {
			throw new Error('网络未连接');
		}
		await ensureCryptoKey();
		const plain = JSON.stringify(payload);
		const wire = await sealWSFrame(plain);
		socket.send(wire);
	}

	/** Wait until WS is OPEN or timeout (network flap). */
	async function waitForOpenSocket(timeoutMs = 12_000): Promise<WebSocket> {
		const deadline = Date.now() + timeoutMs;
		while (Date.now() < deadline) {
			if (ws && ws.readyState === WebSocket.OPEN) return ws;
			// Kick a reconnect if completely down.
			if (!ws || ws.readyState === WebSocket.CLOSED || ws.readyState === WebSocket.CLOSING) {
				if (!intentionalClose && connectionStatus !== 'connecting' && connectionStatus !== 'reconnecting') {
					connect({ isReconnect: true });
				}
			}
			await new Promise((r) => setTimeout(r, 250));
		}
		throw new Error('网络不稳定，连接超时');
	}

	/**
	 * Send with wait-for-connect + exponential backoff retries.
	 * Used for chat content that must not be lost on brief network blips.
	 */
	async function wsSendReliable(
		payload: unknown,
		optsSend: { attempts?: number; label?: string } = {}
	): Promise<void> {
		const attempts = optsSend.attempts ?? 4;
		let lastErr: Error | null = null;
		for (let i = 0; i < attempts; i++) {
			try {
				const socket = await waitForOpenSocket(i === 0 ? 8_000 : 12_000);
				await wsSendJSON(payload, socket);
				return;
			} catch (e) {
				lastErr = e as Error;
				const backoff = Math.min(4000, 400 * Math.pow(2, i));
				console.warn(
					`[ws] send retry ${i + 1}/${attempts}`,
					optsSend.label ?? '',
					lastErr.message
				);
				if (i < attempts - 1) {
					await new Promise((r) => setTimeout(r, backoff));
				}
			}
		}
		throw lastErr ?? new Error('发送失败');
	}

	function updateMessageStatus(
		id: string,
		status: NonNullable<ChatMessage['send_status']>,
		patch?: Partial<ChatMessage>
	) {
		messages = messages.map((m) =>
			m.id === id ? { ...m, send_status: status, ...patch } : m
		);
	}

	/** After socket is open: presence, groups, history (crypto key already loaded). */
	async function onSocketReady(socket: WebSocket, gen: number) {
		if (gen !== socketGen || ws !== socket) return;

		void refreshOnlineUsers();
		void refreshFriends();
		void refreshIncomingRequests();
		void refreshBlacklist();
		void refreshMyGroups();
		void refreshBalance();

		// Re-join groups so membership survives disconnect / refresh.
		// content "rejoin" + REST rejoin=1 → no "加入到群" notice on every reconnect.
		for (const g of joinedGroups) {
			if (gen !== socketGen || ws !== socket || socket.readyState !== WebSocket.OPEN) return;
			try {
				await wsSendJSON(
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

		if (gen !== socketGen || ws !== socket) return;
		void reloadActiveHistory();
		if (chatMode === 'group' && groupId.trim()) {
			void refreshGroupMembers(groupId.trim());
		}
		// Auto-resend messages that failed during network flap.
		void flushPendingSends();
	}

	function attachNetworkHooks() {
		if (networkHooksAttached || typeof window === 'undefined') return;
		networkHooksAttached = true;

		window.addEventListener('online', () => {
			if (intentionalClose) return;
			if (connectionStatus === 'connected') return;
			console.info('[ws] network online — reconnecting');
			reconnectAttempt = 0;
			clearReconnectTimer();
			connect({ isReconnect: true });
		});

		window.addEventListener('offline', () => {
			connectionStatus = 'disconnected';
			clearReconnectTimer();
		});

		document.addEventListener('visibilitychange', () => {
			if (document.visibilityState !== 'visible' || intentionalClose) return;
			// Only reconnect when we truly have no usable socket. Do not treat
			// "reconnecting" alone as a signal if a socket is already OPEN/CONNECTING.
			const need =
				!ws ||
				ws.readyState === WebSocket.CLOSED ||
				ws.readyState === WebSocket.CLOSING ||
				connectionStatus === 'disconnected';
			if (!need) return;
			console.info('[ws] tab visible — ensure connection');
			reconnectAttempt = 0;
			clearReconnectTimer();
			connect({ isReconnect: true });
		});
	}

	function connect(optsConnect: { isReconnect?: boolean } = {}) {
		// Async body so we can load the AES key *before* opening WS (server frames are sealed).
		// Coalesce concurrent callers onto one in-flight attempt.
		if (connectInFlight) {
			return;
		}
		const run = connectAsync(optsConnect).finally(() => {
			if (connectInFlight === run) connectInFlight = null;
		});
		connectInFlight = run;
		void run;
	}

	async function connectAsync(optsConnect: { isReconnect?: boolean } = {}) {
		attachNetworkHooks();
		intentionalClose = false;

		// Prefer latest token (e.g. after profile edit re-issues JWT).
		const token =
			(typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null) ||
			opts.token;
		if (!token) {
			opts.onUnauthorized?.();
			return;
		}

		// Already open — nothing to do.
		if (ws && ws.readyState === WebSocket.OPEN) {
			connectionStatus = 'connected';
			return;
		}
		// Already connecting — don't open a second socket.
		if (ws && ws.readyState === WebSocket.CONNECTING) {
			connectionStatus = optsConnect.isReconnect ? 'reconnecting' : 'connecting';
			return;
		}

		clearReconnectTimer();
		connectionStatus = optsConnect.isReconnect ? 'reconnecting' : 'connecting';

		// Load frame crypto key before any encrypted traffic arrives.
		try {
			await ensureCryptoKey();
		} catch (err) {
			console.error('[crypto] failed to load message key before WS', err);
			// Still try to connect; OpenFrame will fail until key is available.
		}

		if (intentionalClose) return;

		// Re-check after await: another path may have opened a socket.
		if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
			connectionStatus =
				ws.readyState === WebSocket.OPEN
					? 'connected'
					: optsConnect.isReconnect
						? 'reconnecting'
						: 'connecting';
			return;
		}

		// Drop any prior socket without treating it as a user disconnect.
		clearHeartbeat();
		const prev = ws;
		ws = null;
		if (prev) {
			try {
				prev.onopen = null;
				prev.onmessage = null;
				prev.onerror = null;
				prev.onclose = null;
				prev.close();
			} catch {
				// ignore
			}
		}

		const gen = ++socketGen;
		const socket = new WebSocket(buildWsUrl(token));
		ws = socket;

		socket.onopen = () => {
			if (gen !== socketGen) return;
			reconnectAttempt = 0;
			connectionStatus = 'connected';
			console.info('[ws] connected (frame crypto enabled)');
			startHeartbeat(socket, gen);
			void onSocketReady(socket, gen);
		};

		socket.onmessage = (e) => {
			if (gen !== socketGen) return;
			void (async () => {
				try {
					const text = typeof e.data === 'string' ? e.data : String(e.data);
					// Decrypt full WS frame envelope first.
					const opened = await tryOpenWSFrame(text);
					if (!opened) return;
					const raw = JSON.parse(opened) as
						| ChatMessage
						| PresenceEvent
						| GroupMembersEvent
						| PongMessage
						| RecallEvent
						| FriendEvent
						| { type: string; code?: string; message?: string };
					// Application heartbeat reply.
					if ('type' in raw && raw.type === 'pong') {
						onPong(raw as PongMessage);
						return;
					}
					// Message recall.
					if ('type' in raw && raw.type === 'recall' && 'id' in raw) {
						applyRecall((raw as RecallEvent).id);
						return;
					}
					// WebRTC call signaling (media on LiveKit).
					if ('type' in raw && raw.type === 'call') {
						opts.onCallEvent?.(raw as CallEvent);
						return;
					}
					// Friend invite / accept / remove / block.
					if ('type' in raw && raw.type === 'friend_event') {
						const fe = raw as FriendEvent;
						if (fe.action === 'request') {
							void refreshIncomingRequests();
						} else if (fe.action === 'accepted') {
							void refreshFriends();
							void refreshIncomingRequests();
						} else if (fe.action === 'rejected') {
							void refreshIncomingRequests();
						} else if (fe.action === 'removed' || fe.action === 'blocked') {
							void refreshFriends();
							void refreshIncomingRequests();
							void refreshBlacklist();
							// Close private chat if peer was removed/blocked.
							const peer =
								fe.from_user_id === myUserId ? fe.to_user_id : fe.from_user_id;
							if (peer && chatMode === 'private' && targetUser === peer) {
								messages = [];
								targetUser = '';
							}
						}
						return;
					}
					// Server application error (e.g. not friends / recall denied).
					if ('type' in raw && raw.type === 'error') {
						const err = raw as { message?: string };
						if (err.message) console.warn('[ws] error:', err.message);
						return;
					}
					// Real-time online/offline push from server.
					if ('type' in raw && raw.type === 'presence' && 'user_id' in raw) {
						applyPresence(raw as PresenceEvent);
						// Refresh friend online flags when presence changes.
						void refreshFriends();
						return;
					}
					// Group dissolved by owner.
					if ('type' in raw && raw.type === 'group_dissolved' && 'group_id' in raw) {
						const ge = raw as GroupDissolvedEvent;
						const gid = String(ge.group_id ?? '');
						if (gid) {
							joinedGroups = joinedGroups.filter((g) => g !== gid);
							saveJoinedGroups(joinedGroups);
							const nextMeta = { ...groupMeta };
							delete nextMeta[gid];
							groupMeta = nextMeta;
							if (chatMode === 'group' && groupId === gid) {
								messages = [];
								groupId = '';
								groupMembers = [];
							}
						}
						return;
					}
					// Group roster updates (join / leave / disconnect).
					if ('type' in raw && raw.type === 'group_members' && 'group_id' in raw) {
						applyGroupMembers(raw as GroupMembersEvent);
						return;
					}
					// Offline inbox flushed after reconnect.
					if ('type' in raw && raw.type === 'offline_sync') {
						const oe = raw as OfflineSyncEvent;
						console.info('[ws] offline sync', oe.count);
						void refreshBalance();
						return;
					}
					// Typing indicator (ephemeral).
					if ('type' in raw && raw.type === 'typing') {
						applyTypingEvent(raw as TypingEvent);
						return;
					}
					// Red packet claimed by someone.
					if ('type' in raw && raw.type === 'red_packet_claimed') {
						const ev = raw as RedPacketClaimedEvent;
						opts.onRedPacketClaimed?.(ev);
						if (ev.user_id === myUserId) {
							void refreshBalance();
						}
						return;
					}
					const msg = raw as ChatMessage;
					if (msg.recalled && msg.id) {
						applyRecall(msg.id);
						return;
					}
					if (!isChatContent(msg) && !msg.recalled) return;
					void handleIncomingChat(msg);
				} catch {
					// ignore non-JSON / decrypt errors
				}
			})();
		};

		socket.onclose = (ev) => {
			if (gen !== socketGen) return;
			clearHeartbeat();
			ws = null;
			// Self is offline from others' POV after disconnect.
			onlineUsers = onlineUsers.filter((u) => u.user_id !== myUserId);
			groupMembers = groupMembers.filter((u) => u.user_id !== myUserId);

			if (intentionalClose) {
				connectionStatus = 'disconnected';
				return;
			}
			// Auth failures often come as 1008 / 4401-style — try once then give up if no token.
			const tokenNow =
				(typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null) ||
				opts.token;
			if (!tokenNow) {
				connectionStatus = 'disconnected';
				opts.onUnauthorized?.();
				return;
			}
			console.info(`[ws] closed code=${ev.code} reason=${ev.reason || '-'} — will reconnect`);
			scheduleReconnect(`close ${ev.code}`);
		};

		socket.onerror = () => {
			if (gen !== socketGen) return;
			// onclose will follow; mark reconnecting if we plan to retry.
			if (!intentionalClose) {
				connectionStatus = 'reconnecting';
			}
		};
	}

	/** Intentional disconnect — stops auto-reconnect (logout). */
	function disconnect() {
		intentionalClose = true;
		clearReconnectTimer();
		clearHeartbeat();
		reconnectGen++;
		socketGen++;
		reconnectAttempt = 0;
		connectInFlight = null;
		const s = ws;
		ws = null;
		if (s) {
			try {
				s.onopen = null;
				s.onmessage = null;
				s.onerror = null;
				s.onclose = null;
				s.close();
			} catch {
				// ignore
			}
		}
		connectionStatus = 'disconnected';
	}

	/** Force a reconnect now (e.g. after profile token change). */
	function reconnectNow() {
		intentionalClose = false;
		reconnectAttempt = 0;
		clearReconnectTimer();
		clearHeartbeat();
		connectInFlight = null;
		// Tear down existing socket then open fresh.
		const s = ws;
		ws = null;
		socketGen++;
		if (s) {
			try {
				s.onopen = null;
				s.onmessage = null;
				s.onerror = null;
				s.onclose = null;
				s.close();
			} catch {
				// ignore
			}
		}
		connect({ isReconnect: true });
	}

	function activeCacheKey(): string {
		if (chatMode === 'private' && targetUser.trim()) return convKeyPrivate(targetUser.trim());
		if (chatMode === 'group' && groupId.trim()) return convKeyGroup(groupId.trim());
		return '';
	}

	function persistActiveCache(list: ChatMessage[] = messages) {
		const key = activeCacheKey();
		if (!key) return;
		saveConvCache(key, list, maxSeqOf(list));
	}

	function appendMessage(msg: ChatMessage) {
		// Same id: merge (server echo after optimistic send → mark sent).
		if (msg.id) {
			const idx = messages.findIndex((m) => m.id === msg.id);
			if (idx >= 0) {
				const prev = messages[idx];
				const merged: ChatMessage = {
					...prev,
					...msg,
					seq: msg.seq || prev.seq,
					// Keep local plaintext & status for own bubbles unless echo confirms.
					content:
						prev.from === myUserId && prev._local_plain
							? prev._local_plain
							: msg.content || prev.content,
					encrypted: prev.from === myUserId ? false : msg.encrypted,
					_local_plain: prev._local_plain,
					send_status:
						prev.from === myUserId
							? msg.send_status ??
								(prev.send_status === 'sending' || prev.send_status === 'pending'
									? 'sent'
									: prev.send_status) ??
								'sent'
							: undefined
				};
				const next = [...messages];
				next[idx] = merged;
				messages = sortMessagesBySeq(next);
				updatePreview(merged);
				persistActiveCache(messages);
				return;
			}
		}
		const key = messageKey(msg);
		if (messages.some((m) => messageKey(m) === key)) return;
		messages = sortMessagesBySeq([...messages, msg]);
		updatePreview(msg);
		persistActiveCache(messages);
	}

	function mergeMessages(incoming: ChatMessage[]) {
		if (!incoming.length) return;
		const merged = mergeById(messages, incoming);
		messages = merged;
		for (const m of incoming) updatePreview(m);
		persistActiveCache(merged);
	}

	function applyRecall(id: string) {
		if (!id) return;
		messages = messages.map((m) =>
			m.id === id
				? {
						...m,
						recalled: true,
						content: '',
						media_url: undefined,
						encrypted: false
					}
				: m
		);
	}

	async function refreshFriends() {
		try {
			const res = await friendService.listFriends();
			friends = res.friends ?? [];
			for (const f of friends) {
				rememberUsers([{ user_id: f.user_id, username: f.username }]);
			}
		} catch {
			// ignore
		}
	}

	async function refreshIncomingRequests() {
		try {
			const res = await friendService.listIncoming();
			incomingRequests = res.requests ?? [];
		} catch {
			// ignore
		}
	}

	async function inviteFriend(username: string) {
		const name = username.trim();
		if (!name) throw new Error('Enter a username');
		const req = await friendService.invite({ username: name });
		return req;
	}

	async function acceptFriendRequest(id: number) {
		const req = await friendService.accept(id);
		incomingRequests = incomingRequests.filter((r) => r.id !== id);
		await refreshFriends();
		return req;
	}

	async function rejectFriendRequest(id: number) {
		await friendService.reject(id);
		incomingRequests = incomingRequests.filter((r) => r.id !== id);
	}

	async function removeFriend(userId: string) {
		await friendService.remove(userId);
		friends = friends.filter((f) => f.user_id !== userId);
		if (chatMode === 'private' && targetUser === userId) {
			messages = [];
			targetUser = '';
		}
	}

	async function refreshBlacklist() {
		try {
			const res = await friendService.listBlacklist();
			blacklist = res.blacklist ?? [];
			for (const u of blacklist) {
				rememberUsers([{ user_id: u.user_id, username: u.username }]);
			}
		} catch {
			// ignore
		}
	}

	/** 拉黑：解除好友 + 进黑名单 */
	async function blockUser(opts: { user_id?: string; username?: string }) {
		const entry = await friendService.block(opts);
		const uid = entry.user_id;
		friends = friends.filter((f) => f.user_id !== uid);
		incomingRequests = incomingRequests.filter(
			(r) => r.from_user_id !== uid && r.to_user_id !== uid
		);
		await refreshBlacklist();
		if (chatMode === 'private' && targetUser === uid) {
			messages = [];
			targetUser = '';
		}
		return entry;
	}

	async function unblockUser(userId: string) {
		await friendService.unblock(userId);
		blacklist = blacklist.filter((u) => u.user_id !== userId);
	}

	async function reloadActiveHistory() {
		if (chatMode === 'private' && targetUser.trim()) {
			await loadPrivateHistory(targetUser.trim(), true);
		} else if (chatMode === 'group' && groupId.trim()) {
			await loadGroupHistory(groupId.trim(), true);
		}
	}

	/**
	 * Load conversation history:
	 *  1) Hydrate from local cache immediately (ordered by seq)
	 *  2) Fetch only messages with seq > max_seq (or full window if no cache)
	 *  3) Merge, sort, write cache
	 */
	async function loadPrivateHistory(peerId: string, force = false) {
		const key = convKeyPrivate(peerId);
		if (!force && loadedKey === key) return;
		const epoch = ++historyEpoch;
		const switching = loadedKey !== key;
		loadedKey = key;
		historyLoading = true;

		// 1) Cache first paint
		const cached = loadConvCache(key);
		let base: ChatMessage[] = [];
		if (cached?.messages?.length) {
			base = sortMessagesBySeq([...cached.messages]);
			if (switching || messages.length === 0) {
				messages = base;
				for (const m of base) updatePreview(m);
			} else {
				base = mergeById(messages, base);
				messages = base;
			}
		} else if (switching) {
			messages = [];
		} else {
			base = [...messages];
		}

		try {
			await ensureCryptoKey().catch(() => undefined);
			const sinceSeq = maxSeqOf(base);
			// Full window when no seq yet; otherwise delta only after last seq.
			const res = await chatService.getPrivateHistory(peerId, sinceSeq > 0 ? 100 : 100, {
				sinceSeq: sinceSeq > 0 ? sinceSeq : undefined
			});
			if (epoch !== historyEpoch) return;
			const list = await decryptMessages((res.messages ?? []).filter(isChatContent));
			if (list.length || sinceSeq === 0) {
				const merged = mergeById(base, list);
				messages = merged;
				for (const m of list) updatePreview(m);
				const maxSeq = Math.max(sinceSeq, res.max_seq ?? 0, maxSeqOf(merged));
				saveConvCache(key, merged, maxSeq);
			} else {
				// Empty delta — still refresh cache max_seq
				saveConvCache(key, base, Math.max(sinceSeq, res.max_seq ?? 0));
			}
		} catch {
			if (epoch === historyEpoch && !base.length) messages = [];
		} finally {
			if (epoch === historyEpoch) historyLoading = false;
		}
	}

	async function loadGroupHistory(g: string, force = false) {
		const key = convKeyGroup(g);
		if (!force && loadedKey === key) return;
		const epoch = ++historyEpoch;
		const switching = loadedKey !== key;
		loadedKey = key;
		historyLoading = true;

		const cached = loadConvCache(key);
		let base: ChatMessage[] = [];
		if (cached?.messages?.length) {
			base = sortMessagesBySeq([...cached.messages]);
			if (switching || messages.length === 0) {
				messages = base;
				for (const m of base) updatePreview(m);
			} else {
				base = mergeById(messages, base);
				messages = base;
			}
		} else if (switching) {
			messages = [];
		} else {
			base = [...messages];
		}

		try {
			await ensureCryptoKey().catch(() => undefined);
			const sinceSeq = maxSeqOf(base);
			const res = await chatService.getGroupHistory(g, 100, {
				sinceSeq: sinceSeq > 0 ? sinceSeq : undefined
			});
			if (epoch !== historyEpoch) return;
			const list = await decryptMessages((res.messages ?? []).filter(isChatContent));
			if (list.length || sinceSeq === 0) {
				const merged = mergeById(base, list);
				messages = merged;
				for (const m of list) updatePreview(m);
				const maxSeq = Math.max(sinceSeq, res.max_seq ?? 0, maxSeqOf(merged));
				saveConvCache(key, merged, maxSeq);
			} else {
				saveConvCache(key, base, Math.max(sinceSeq, res.max_seq ?? 0));
			}
		} catch {
			if (epoch === historyEpoch && !base.length) messages = [];
		} finally {
			if (epoch === historyEpoch) historyLoading = false;
		}
	}

	async function refreshOnlineUsers() {
		try {
			const res = await chatService.getOnlineUsers();
			// Replace entire list from server (authoritative hub-based snapshot).
			const list = normalizeOnlineList(res.online_users);
			onlineUsers = list;
			rememberUsers(list);
		} catch {
			// ignore
		}
	}

	async function refreshGroupMembers(g?: string) {
		const gid = (g ?? groupId).trim();
		if (!gid) {
			groupMembers = [];
			return;
		}
		try {
			const res = await groupService.members(gid);
			// Only apply if still viewing this group.
			if (groupId.trim() !== gid) return;
			const list = normalizeOnlineList(res.members);
			groupMembers = list;
			rememberUsers(list);
		} catch {
			if (groupId.trim() === gid) groupMembers = [];
		}
	}

	function displayName(userId: string): string {
		if (!userId) return '';
		return userLabels[userId] || userId;
	}

	function conversationTarget(): { peer: string; g: string } | null {
		const peer = targetUser.trim();
		const g = groupId.trim();
		if (chatMode === 'private' && !peer) return null;
		if (chatMode === 'group' && !g) return null;
		return { peer, g };
	}

	async function sendMessage() {
		if (!inputText.trim()) return;

		const dest = conversationTarget();
		if (!dest) {
			toastInfo(chatMode === 'private' ? '请先选择好友' : '请先选择群聊');
			return;
		}

		const plain = inputText.trim();
		const id = newMsgId();
		const ts = Math.floor(Date.now() / 1000);

		// Optimistic bubble immediately — shows "发送中…" while network recovers.
		const local: ChatMessage = {
			id,
			type: chatMode,
			from: myUserId,
			to: chatMode === 'private' ? dest.peer : dest.g,
			content: plain,
			encrypted: false,
			content_type: 'text',
			group_id: chatMode === 'group' ? dest.g : '',
			timestamp: ts,
			send_status: 'sending',
			_local_plain: plain
		};
		appendMessage(local);
		inputText = '';
		// Sending a message ends our typing state.
		notifyTypingStop();

		void deliverChatMessage(local);
	}

	/** Encrypt + reliable WS send; updates bubble status. */
	async function deliverChatMessage(local: ChatMessage) {
		if (!local.id) return;
		updateMessageStatus(local.id, 'sending');

		try {
			await ensureCryptoKey();
		} catch (err) {
			console.error('[crypto] key load failed', err);
			updateMessageStatus(local.id, 'failed');
			toastError('加密密钥不可用，无法发送');
			return;
		}

		const plain = local._local_plain ?? local.content;
		let cipher = plain;
		try {
			// Voice / red_packet: content may already be label; re-encrypt plaintext label.
			if (local.content_type === 'voice') {
				cipher = await encryptContent(plain || '🎤 Voice message');
			} else if (local.content_type === 'red_packet') {
				// Red packets are published by REST — not re-sent here.
				updateMessageStatus(local.id, 'sent');
				return;
			} else {
				cipher = await encryptContent(plain);
			}
		} catch (err) {
			console.error('[crypto] encrypt failed', err);
			updateMessageStatus(local.id, 'failed');
			toastError('消息加密失败');
			return;
		}

		const wire: ChatMessage = {
			id: local.id,
			type: local.type,
			from: local.from,
			to: local.to,
			content: cipher,
			encrypted: true,
			content_type: local.content_type ?? 'text',
			media_url: local.media_url,
			duration: local.duration,
			group_id: local.group_id ?? '',
			timestamp: local.timestamp ?? Math.floor(Date.now() / 1000),
			red_packet_id: local.red_packet_id
		};

		try {
			await wsSendReliable(wire, { attempts: 4, label: local.id });
			updateMessageStatus(local.id, 'sent');
		} catch (err) {
			console.error('[ws] deliver failed', err);
			updateMessageStatus(local.id, 'failed');
			toastError((err as Error).message || '发送失败，可点击重试');
		}
	}

	/** Manual / auto resend of a failed outbound message. */
	async function resendMessage(msg: ChatMessage) {
		if (!msg.id || msg.from !== myUserId) return;
		if (msg.send_status !== 'failed' && msg.send_status !== 'pending') return;
		const local: ChatMessage = {
			...msg,
			send_status: 'sending',
			_local_plain: msg._local_plain ?? msg.content
		};
		// Keep bubble content as plaintext for UI.
		messages = messages.map((m) =>
			m.id === msg.id
				? {
						...m,
						send_status: 'sending',
						content: local._local_plain ?? m.content,
						encrypted: false
					}
				: m
		);
		toastInfo('正在重新发送…');
		await deliverChatMessage(local);
	}

	/** Flush any failed/pending own messages after reconnect. */
	async function flushPendingSends() {
		const pending = messages.filter(
			(m) =>
				m.from === myUserId &&
				m.id &&
				(m.send_status === 'failed' || m.send_status === 'pending')
		);
		for (const m of pending) {
			await resendMessage(m);
		}
	}

	/** Recall own message within the server window (2 minutes). */
	async function recallMessage(msg: ChatMessage) {
		if (!msg.id) return;
		if (msg.from !== myUserId || msg.recalled) return;
		if (msg.send_status === 'sending' || msg.send_status === 'failed') {
			toastInfo('消息尚未送达，无法撤回');
			return;
		}
		try {
			await wsSendReliable(
				{
					type: 'recall',
					id: msg.id,
					from: myUserId,
					to: msg.to,
					content: '',
					group_id: msg.group_id ?? ''
				} satisfies ChatMessage,
				{ attempts: 3, label: `recall:${msg.id}` }
			);
			// Optimistic mark; server broadcast confirms for peers.
			applyRecall(msg.id);
		} catch (err) {
			toastError((err as Error).message || '撤回失败');
		}
	}

	/** Upload recorded audio and send as a voice chat message. */
	async function sendRedPacket(optsSend: {
		total_amount: number;
		total_count?: number;
		greeting?: string;
	}) {
		const dest = conversationTarget();
		if (!dest) throw new Error('Select a conversation first');
		const body: CreateRedPacketBody =
			chatMode === 'private'
				? {
						type: 'private',
						peer_id: dest.peer,
						total_amount: optsSend.total_amount,
						greeting: optsSend.greeting
					}
				: {
						type: 'group',
						group_id: dest.g,
						total_amount: optsSend.total_amount,
						total_count: optsSend.total_count ?? 1,
						greeting: optsSend.greeting
					};
		const res = await redPacketService.create(body);
		const msg = res.message as ChatMessage;
		if (msg && isChatContent(msg)) {
			const plain = await decryptMessage(msg);
			if (belongsToConversation(plain, chatMode, myUserId, targetUser, groupId)) {
				appendMessage(plain);
			} else {
				updatePreview(plain);
			}
		}
		await refreshBalance();
		return res;
	}

	async function sendVoiceMessage(blob: Blob, durationSec: number) {
		const dest = conversationTarget();
		if (!dest) {
			throw new Error(chatMode === 'private' ? '请先选择好友' : '请先选择群聊');
		}
		if (blob.size === 0) {
			throw new Error('录音为空');
		}

		// Upload may use REST (works offline from WS); then queue WS deliver with retry.
		const uploaded = await mediaService.uploadVoice(blob, durationSec);
		const plainLabel = '🎤 语音消息';
		const id = newMsgId();
		const local: ChatMessage = {
			id,
			type: chatMode,
			from: myUserId,
			to: chatMode === 'private' ? dest.peer : dest.g,
			content: plainLabel,
			encrypted: false,
			content_type: 'voice',
			media_url: uploaded.url,
			duration: durationSec > 0 ? durationSec : uploaded.duration,
			group_id: chatMode === 'group' ? dest.g : '',
			timestamp: Math.floor(Date.now() / 1000),
			send_status: 'sending',
			_local_plain: plainLabel
		};
		appendMessage(local);
		await deliverChatMessage(local);
		if (messages.find((m) => m.id === id)?.send_status === 'failed') {
			throw new Error('语音已上传，但发送失败，可点击重试');
		}
	}

	async function refreshMyGroups() {
		try {
			const res = await groupService.listMine();
			const list = res.groups ?? [];
			const ids: string[] = [];
			const meta: Record<string, GroupInfo> = {};
			for (const g of list) {
				if (!g?.id) continue;
				ids.push(g.id);
				meta[g.id] = g;
			}
			joinedGroups = ids;
			groupMeta = meta;
			saveJoinedGroups(ids);
		} catch {
			// keep local cache
		}
	}

	async function createGroup(name?: string, customId?: string) {
		const g = await groupService.create({
			name: name?.trim() || undefined,
			group_id: customId?.trim() || undefined
		});
		joinedGroups = [...new Set([...joinedGroups, g.id])];
		groupMeta = { ...groupMeta, [g.id]: g };
		saveJoinedGroups(joinedGroups);
		if (ws?.readyState === WebSocket.OPEN) {
			try {
				await wsSendJSON({
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
		chatMode = 'group';
		groupId = g.id;
		await Promise.all([loadGroupHistory(g.id), refreshGroupMembers(g.id)]);
		return g;
	}

	async function dissolveGroup(g: string) {
		const id = g.trim();
		if (!id) return;
		await groupService.dissolve(id);
		joinedGroups = joinedGroups.filter((g2) => g2 !== id);
		saveJoinedGroups(joinedGroups);
		const nextMeta = { ...groupMeta };
		delete nextMeta[id];
		groupMeta = nextMeta;
		if (chatMode === 'group' && groupId === id) {
			messages = [];
			groupId = '';
			groupMembers = [];
		}
	}

	async function joinGroup() {
		const g = groupId.trim();
		if (!g) return;
		try {
			await groupService.join(g);
			joinedGroups = [...new Set([...joinedGroups, g])];
			saveJoinedGroups(joinedGroups);
			if (ws?.readyState === WebSocket.OPEN) {
				await wsSendJSON({
					type: 'join_group',
					from: myUserId,
					to: g,
					content: '',
					group_id: g
				} satisfies ChatMessage);
			}
			chatMode = 'group';
			groupId = g;
			await Promise.all([loadGroupHistory(g), refreshGroupMembers(g), refreshMyGroups()]);
		} catch (err) {
			toastError((err as Error).message || '加入群失败');
		}
	}

	async function leaveGroup(g: string) {
		try {
			await groupService.leave(g);
			joinedGroups = joinedGroups.filter((g2) => g2 !== g);
			saveJoinedGroups(joinedGroups);
			const nextMeta = { ...groupMeta };
			delete nextMeta[g];
			groupMeta = nextMeta;
			if (ws?.readyState === WebSocket.OPEN) {
				await wsSendJSON({
					type: 'leave_group',
					from: myUserId,
					to: g,
					content: '',
					group_id: g
				} satisfies ChatMessage);
			}
			if (chatMode === 'group' && groupId === g) {
				messages = [];
				groupId = '';
				groupMembers = [];
			}
		} catch (err) {
			toastError((err as Error).message || '退出群失败');
		}
	}

	function groupDisplayName(id: string): string {
		return groupMeta[id]?.name || id;
	}

	function isGroupOwner(id: string): boolean {
		return groupMeta[id]?.role === 'owner' || groupMeta[id]?.owner_user_id === myUserId;
	}

	async function selectPrivateUser(userId: string, username?: string) {
		clearUnread(userId);
		const peer = String(userId ?? '').trim();
		if (!peer || peer === myUserId) return;

		// Switch to private + clear blink first so UI goes normal immediately on click.
		notifyTypingStop();
		chatMode = 'private';
		targetUser = peer;
		clearUnread(peer);
		clearTypingForConversation();

		if (username) {
			rememberUsers([{ user_id: peer, username }]);
		}
		ensurePeerListed(peer, username);

		// Keep last groupId for when user returns to Group tab, but open private now.
		await loadPrivateHistory(peer);
		// Clear again after history in case a message raced in during load.
		clearUnread(peer);
	}

	async function selectGroup(g: string) {
		notifyTypingStop();
		chatMode = 'group';
		groupId = g;
		groupMembers = [];
		clearGroupUnread(g);
		clearTypingForConversation();
		// First time in local list → announce; already joined → silent rejoin.
		const firstJoin = !joinedGroups.includes(g);
		if (firstJoin) {
			try {
				await groupService.join(g);
				joinedGroups = [...new Set([...joinedGroups, g])];
				saveJoinedGroups(joinedGroups);
			} catch {
				// still try to load history
			}
		}
		if (ws?.readyState === WebSocket.OPEN) {
			try {
				await wsSendJSON({
					type: 'join_group',
					from: myUserId,
					to: g,
					content: firstJoin ? '' : 'rejoin',
					group_id: g
				} satisfies ChatMessage);
			} catch (err) {
				console.error('[ws] join_group send failed', err);
			}
		} else if (!firstJoin) {
			// Offline rejoin path when WS not ready yet — membership restored on connect.
		}
		await Promise.all([loadGroupHistory(g), refreshGroupMembers(g)]);
	}

	function setChatMode(mode: ChatMode) {
		if (chatMode === mode) return;
		notifyTypingStop();
		chatMode = mode;
		messages = [];
		loadedKey = '';
		clearTypingForConversation();
		if (mode === 'group' && groupId.trim()) {
			void refreshGroupMembers(groupId.trim());
		} else if (mode === 'private') {
			// Keep last group members only while in group mode.
			groupMembers = [];
		}
		void reloadActiveHistory();
	}

	return {
		get messages() {
			return messages;
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
		get joinedGroups() {
			return joinedGroups;
		},
		get groupMeta() {
			return groupMeta;
		},
		get onlineUsers() {
			return onlineUsers;
		},
		get friends() {
			return friends;
		},
		get incomingRequests() {
			return incomingRequests;
		},
		get blacklist() {
			return blacklist;
		},
		get groupMembers() {
			return groupMembers;
		},
		get unreadPeers() {
			return unreadPeers;
		},
		get unreadGroups() {
			return unreadGroups;
		},
		get lastPreviews() {
			return lastPreviews;
		},
		get balance() {
			return balance;
		},
		get typingUsers() {
			return typingUsers;
		},
		get typingHint() {
			return typingHint;
		},
		get connectionStatus() {
			return connectionStatus;
		},
		get reconnectAttempt() {
			return reconnectAttempt;
		},
		get myUserId() {
			return myUserId;
		},
		get historyLoading() {
			return historyLoading;
		},
		displayName,
		hasUnread,
		connect,
		disconnect,
		reconnectNow,
		refreshOnlineUsers,
		refreshFriends,
		refreshIncomingRequests,
		refreshGroupMembers,
		inviteFriend,
		acceptFriendRequest,
		rejectFriendRequest,
		removeFriend,
		refreshBlacklist,
		blockUser,
		unblockUser,
		sendMessage,
		sendVoiceMessage,
		sendRedPacket,
		resendMessage,
		refreshBalance,
		hasGroupUnread,
		notifyTyping,
		notifyTypingStop,
		typingLabel,
		recallMessage,
		joinGroup,
		leaveGroup,
		createGroup,
		dissolveGroup,
		refreshMyGroups,
		groupDisplayName,
		isGroupOwner,
		selectPrivateUser,
		selectGroup,
		setChatMode,
		loadPrivateHistory,
		loadGroupHistory
	};
}

export type ChatController = ReturnType<typeof createChatController>;
