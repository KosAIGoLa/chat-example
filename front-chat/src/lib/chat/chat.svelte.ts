import { api, buildWsUrl } from '$lib/api';
import {
	encryptContent,
	hasMessageKey,
	importMessageKey,
	isEncryptedContent,
	sealWSFrame,
	tryDecryptContent,
	tryOpenWSFrame
} from './crypto';
import type {
	ChatMessage,
	ChatMode,
	ConnectionStatus,
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
	RecallEvent
} from './types';

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
	/** Members of the currently selected group (never includes self). */
	let groupMembers = $state<OnlineUser[]>([]);
	/** user_id → username cache for titles / labels. */
	let userLabels = $state<Record<string, string>>({});
	/** user_ids with unread private messages (blink in list). */
	let unreadPeers = $state<Record<string, boolean>>({});
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

		if (!belongsToConversation(plain, chatMode, myUserId, targetUser, groupId)) {
			return;
		}
		appendMessage(plain);
	}

	async function ensureCryptoKey(): Promise<void> {
		if (hasMessageKey()) return;
		const res = await api.getCryptoKey();
		await importMessageKey(res.key);
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
			throw new Error('Not connected');
		}
		await ensureCryptoKey();
		const plain = JSON.stringify(payload);
		const wire = await sealWSFrame(plain);
		socket.send(wire);
	}

	/** After socket is open: presence, groups, history (crypto key already loaded). */
	async function onSocketReady(socket: WebSocket, gen: number) {
		if (gen !== socketGen || ws !== socket) return;

		void refreshOnlineUsers();
		void refreshFriends();
		void refreshIncomingRequests();
		void refreshMyGroups();

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
			void api.joinGroup(g, { rejoin: true }).catch(() => {
				// REST join needs WS online; ignore race — WS join_group still applies.
			});
		}

		if (gen !== socketGen || ws !== socket) return;
		void reloadActiveHistory();
		if (chatMode === 'group' && groupId.trim()) {
			void refreshGroupMembers(groupId.trim());
		}
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
					// Friend invite / accept.
					if ('type' in raw && raw.type === 'friend_event') {
						const fe = raw as FriendEvent;
						if (fe.action === 'request') {
							void refreshIncomingRequests();
						} else if (fe.action === 'accepted') {
							void refreshFriends();
							void refreshIncomingRequests();
						} else if (fe.action === 'rejected') {
							// optional toast
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

	function appendMessage(msg: ChatMessage) {
		const key = messageKey(msg);
		if (messages.some((m) => messageKey(m) === key)) return;
		// If same id already present (optimistic + server echo), skip.
		if (msg.id && messages.some((m) => m.id === msg.id)) return;
		messages = [...messages, msg];
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
			const res = await api.listFriends();
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
			const res = await api.listIncomingFriendRequests();
			incomingRequests = res.requests ?? [];
		} catch {
			// ignore
		}
	}

	async function inviteFriend(username: string) {
		const name = username.trim();
		if (!name) throw new Error('Enter a username');
		const req = await api.inviteFriend({ username: name });
		return req;
	}

	async function acceptFriendRequest(id: number) {
		const req = await api.acceptFriendRequest(id);
		incomingRequests = incomingRequests.filter((r) => r.id !== id);
		await refreshFriends();
		return req;
	}

	async function rejectFriendRequest(id: number) {
		await api.rejectFriendRequest(id);
		incomingRequests = incomingRequests.filter((r) => r.id !== id);
	}

	async function removeFriend(userId: string) {
		await api.removeFriend(userId);
		friends = friends.filter((f) => f.user_id !== userId);
		if (chatMode === 'private' && targetUser === userId) {
			messages = [];
			targetUser = '';
		}
	}

	async function reloadActiveHistory() {
		if (chatMode === 'private' && targetUser.trim()) {
			await loadPrivateHistory(targetUser.trim(), true);
		} else if (chatMode === 'group' && groupId.trim()) {
			await loadGroupHistory(groupId.trim(), true);
		}
	}

	async function loadPrivateHistory(peerId: string, force = false) {
		const key = `private:${peerId}`;
		if (!force && loadedKey === key) return;
		const epoch = ++historyEpoch;
		loadedKey = key;
		historyLoading = true;
		messages = [];
		try {
			await ensureCryptoKey().catch(() => undefined);
			const res = await api.getPrivateHistory(peerId);
			if (epoch !== historyEpoch) return;
			const list = (res.messages ?? []).filter(isChatContent);
			messages = await decryptMessages(list);
		} catch {
			if (epoch === historyEpoch) messages = [];
		} finally {
			if (epoch === historyEpoch) historyLoading = false;
		}
	}

	async function loadGroupHistory(g: string, force = false) {
		const key = `group:${g}`;
		if (!force && loadedKey === key) return;
		const epoch = ++historyEpoch;
		loadedKey = key;
		historyLoading = true;
		messages = [];
		try {
			await ensureCryptoKey().catch(() => undefined);
			const res = await api.getGroupHistory(g);
			if (epoch !== historyEpoch) return;
			const list = (res.messages ?? []).filter(isChatContent);
			messages = await decryptMessages(list);
		} catch {
			if (epoch === historyEpoch) messages = [];
		} finally {
			if (epoch === historyEpoch) historyLoading = false;
		}
	}

	async function refreshOnlineUsers() {
		try {
			const res = await api.getOnlineUsers();
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
			const res = await api.getGroupMembers(gid);
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
		if (!inputText.trim() || !ws || ws.readyState !== WebSocket.OPEN) return;

		const dest = conversationTarget();
		if (!dest) return;

		const plain = inputText.trim();
		try {
			await ensureCryptoKey();
		} catch (err) {
			console.error('[crypto] key load failed', err);
			alert('Encryption key not available — cannot send');
			return;
		}

		let cipher = plain;
		try {
			cipher = await encryptContent(plain);
		} catch (err) {
			console.error('[crypto] encrypt failed', err);
			alert('Failed to encrypt message');
			return;
		}

		const wire: ChatMessage = {
			id: newMsgId(),
			type: chatMode,
			from: myUserId,
			to: chatMode === 'private' ? dest.peer : dest.g,
			content: cipher,
			encrypted: true,
			content_type: 'text',
			group_id: chatMode === 'group' ? dest.g : '',
			timestamp: Math.floor(Date.now() / 1000)
		};

		try {
			await wsSendJSON(wire);
		} catch (err) {
			alert((err as Error).message || 'Send failed');
			return;
		}
		// Optimistic local echo with plaintext for the sender UI.
		appendMessage({ ...wire, content: plain, encrypted: false });
		inputText = '';
	}

	/** Recall own message within the server window (2 minutes). */
	async function recallMessage(msg: ChatMessage) {
		if (!msg.id || !ws || ws.readyState !== WebSocket.OPEN) return;
		if (msg.from !== myUserId || msg.recalled) return;
		try {
			await wsSendJSON({
				type: 'recall',
				id: msg.id,
				from: myUserId,
				to: msg.to,
				content: '',
				group_id: msg.group_id ?? ''
			} satisfies ChatMessage);
			// Optimistic mark; server broadcast confirms for peers.
			applyRecall(msg.id);
		} catch (err) {
			alert((err as Error).message || 'Recall failed');
		}
	}

	/** Upload recorded audio and send as a voice chat message. */
	async function sendVoiceMessage(blob: Blob, durationSec: number) {
		if (!ws || ws.readyState !== WebSocket.OPEN) {
			throw new Error('Not connected');
		}
		const dest = conversationTarget();
		if (!dest) {
			throw new Error(chatMode === 'private' ? 'Select a user first' : 'Select a group first');
		}
		if (blob.size === 0) {
			throw new Error('Empty recording');
		}

		await ensureCryptoKey();
		const uploaded = await api.uploadVoice(blob, durationSec);
		const plainLabel = '🎤 Voice message';
		const cipher = await encryptContent(plainLabel);
		const wire: ChatMessage = {
			id: newMsgId(),
			type: chatMode,
			from: myUserId,
			to: chatMode === 'private' ? dest.peer : dest.g,
			content: cipher,
			encrypted: true,
			content_type: 'voice',
			media_url: uploaded.url,
			duration: durationSec > 0 ? durationSec : uploaded.duration,
			group_id: chatMode === 'group' ? dest.g : '',
			timestamp: Math.floor(Date.now() / 1000)
		};

		await wsSendJSON(wire);
		appendMessage({ ...wire, content: plainLabel, encrypted: false });
	}

	async function refreshMyGroups() {
		try {
			const res = await api.listMyGroups();
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
		const g = await api.createGroup({
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
		await api.dissolveGroup(id);
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
			await api.joinGroup(g);
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
			alert((err as Error).message);
		}
	}

	async function leaveGroup(g: string) {
		try {
			await api.leaveGroup(g);
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
			alert((err as Error).message);
		}
	}

	function groupDisplayName(id: string): string {
		return groupMeta[id]?.name || id;
	}

	function isGroupOwner(id: string): boolean {
		return groupMeta[id]?.role === 'owner' || groupMeta[id]?.owner_user_id === myUserId;
	}

	async function selectPrivateUser(userId: string, username?: string) {
		const peer = String(userId ?? '').trim();
		if (!peer || peer === myUserId) return;

		// Switch to private + clear blink first so UI goes normal immediately on click.
		chatMode = 'private';
		targetUser = peer;
		clearUnread(peer);

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
		chatMode = 'group';
		groupId = g;
		groupMembers = [];
		// First time in local list → announce; already joined → silent rejoin.
		const firstJoin = !joinedGroups.includes(g);
		if (firstJoin) {
			try {
				await api.joinGroup(g);
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
		chatMode = mode;
		messages = [];
		loadedKey = '';
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
		get groupMembers() {
			return groupMembers;
		},
		get unreadPeers() {
			return unreadPeers;
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
		sendMessage,
		sendVoiceMessage,
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
