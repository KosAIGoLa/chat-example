/**
 * WebSocket session: connect / reconnect / heartbeat / sealed send.
 * Domain handlers stay outside; this only owns the socket lifecycle.
 */

import { buildWsUrl } from '$lib/api';
import {
	hasMessageKey,
	importMessageKeyFromWrapped,
	sealWSFrame,
	tryOpenWSFrame
} from '../crypto';
import { chatService } from '$lib/api';
import type { ConnectionStatus, PingMessage, PongMessage } from '../types';
import {
	HEARTBEAT_INTERVAL_MS,
	HEARTBEAT_TIMEOUT_MS,
	RECONNECT_BASE_MS,
	RECONNECT_MAX_ATTEMPTS,
	RECONNECT_MAX_MS
} from './constants';

export type WsMessageHandler = (raw: unknown) => void | Promise<void>;

export interface WsSessionOpts {
	getToken: () => string | null;
	onUnauthorized?: () => void;
	onStatus: (status: ConnectionStatus, reconnectAttempt?: number) => void;
	onMessage: WsMessageHandler;
	/** Called after OPEN (crypto key already attempted). */
	onOpen: (socket: WebSocket, gen: number) => void | Promise<void>;
	/** Load AES key before opening / sending. */
	ensureCryptoKey?: () => Promise<void>;
}

export function createWsSession(opts: WsSessionOpts) {
	let ws: WebSocket | null = null;
	let intentionalClose = false;
	let reconnectGen = 0;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let socketGen = 0;
	let networkHooksAttached = false;
	let connectInFlight: Promise<void> | null = null;
	let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
	let heartbeatTimeout: ReturnType<typeof setTimeout> | null = null;
	let lastPongAt = 0;
	let reconnectAttempt = 0;
	let connectionStatus: ConnectionStatus = 'disconnected';

	function setStatus(s: ConnectionStatus) {
		connectionStatus = s;
		opts.onStatus(s, reconnectAttempt);
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

	async function ensureCryptoKey(): Promise<void> {
		if (opts.ensureCryptoKey) {
			await opts.ensureCryptoKey();
			return;
		}
		if (hasMessageKey()) return;
		const res = await chatService.getCryptoKey();
		const token = opts.getToken() || '';
		if (!token) throw new Error('missing auth token for crypto key unwrap');
		if (!res?.w) throw new Error('crypto key response missing wrapped blob');
		await importMessageKeyFromWrapped(res.w, token);
	}

	function startHeartbeat(socket: WebSocket, gen: number) {
		clearHeartbeat();
		lastPongAt = Date.now();

		const sendPing = () => {
			if (gen !== socketGen || ws !== socket || socket.readyState !== WebSocket.OPEN) {
				clearHeartbeat();
				return;
			}
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
				try {
					socket.close(4000, 'heartbeat timeout');
				} catch {
					// ignore
				}
			}, HEARTBEAT_TIMEOUT_MS);
		};

		heartbeatTimer = setInterval(sendPing, HEARTBEAT_INTERVAL_MS);
	}

	function onPong(msg: PongMessage) {
		if (heartbeatTimeout != null) {
			clearTimeout(heartbeatTimeout);
			heartbeatTimeout = null;
		}
		lastPongAt = Date.now();
		void msg;
		void lastPongAt;
	}

	function scheduleReconnect(reason: string) {
		if (intentionalClose) return;
		if (reconnectTimer != null) return;
		if (reconnectAttempt >= RECONNECT_MAX_ATTEMPTS) {
			console.error('[ws] max reconnect attempts reached', reason);
			setStatus('disconnected');
			return;
		}
		const delay = Math.min(
			RECONNECT_MAX_MS,
			RECONNECT_BASE_MS * Math.pow(2, Math.min(reconnectAttempt, 5))
		);
		reconnectAttempt += 1;
		setStatus('reconnecting');
		console.info(`[ws] reconnect in ${delay}ms (attempt ${reconnectAttempt}) — ${reason}`);
		const gen = ++reconnectGen;
		reconnectTimer = setTimeout(() => {
			reconnectTimer = null;
			if (gen !== reconnectGen || intentionalClose) return;
			connect({ isReconnect: true });
		}, delay);
	}

	async function wsSendJSON(payload: unknown, socket: WebSocket = ws!): Promise<void> {
		if (!socket || socket.readyState !== WebSocket.OPEN) {
			throw new Error('网络未连接');
		}
		await ensureCryptoKey();
		const plain = JSON.stringify(payload);
		const wire = await sealWSFrame(plain);
		socket.send(wire);
	}

	async function waitForOpenSocket(timeoutMs = 12_000): Promise<WebSocket> {
		const deadline = Date.now() + timeoutMs;
		while (Date.now() < deadline) {
			if (ws && ws.readyState === WebSocket.OPEN) return ws;
			if (!ws || ws.readyState === WebSocket.CLOSED || ws.readyState === WebSocket.CLOSING) {
				if (
					!intentionalClose &&
					connectionStatus !== 'connecting' &&
					connectionStatus !== 'reconnecting'
				) {
					connect({ isReconnect: true });
				}
			}
			await new Promise((r) => setTimeout(r, 250));
		}
		throw new Error('网络不稳定，连接超时');
	}

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
			setStatus('disconnected');
			clearReconnectTimer();
		});

		document.addEventListener('visibilitychange', () => {
			if (document.visibilityState !== 'visible' || intentionalClose) return;
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
		if (connectInFlight) return;
		const run = connectAsync(optsConnect).finally(() => {
			if (connectInFlight === run) connectInFlight = null;
		});
		connectInFlight = run;
		void run;
	}

	async function connectAsync(optsConnect: { isReconnect?: boolean } = {}) {
		attachNetworkHooks();
		intentionalClose = false;

		const token = opts.getToken();
		if (!token) {
			opts.onUnauthorized?.();
			return;
		}

		if (ws && ws.readyState === WebSocket.OPEN) {
			setStatus('connected');
			return;
		}
		if (ws && ws.readyState === WebSocket.CONNECTING) {
			setStatus(optsConnect.isReconnect ? 'reconnecting' : 'connecting');
			return;
		}

		clearReconnectTimer();
		setStatus(optsConnect.isReconnect ? 'reconnecting' : 'connecting');

		try {
			await ensureCryptoKey();
		} catch (err) {
			console.error('[crypto] failed to load message key before WS', err);
		}

		if (intentionalClose) return;

		if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
			setStatus(
				ws.readyState === WebSocket.OPEN
					? 'connected'
					: optsConnect.isReconnect
						? 'reconnecting'
						: 'connecting'
			);
			return;
		}

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
			setStatus('connected');
			console.info('[ws] connected (frame crypto enabled)');
			startHeartbeat(socket, gen);
			void opts.onOpen(socket, gen);
		};

		socket.onmessage = (e) => {
			if (gen !== socketGen) return;
			void (async () => {
				try {
					const text = typeof e.data === 'string' ? e.data : String(e.data);
					const opened = await tryOpenWSFrame(text);
					if (!opened) return;
					const raw = JSON.parse(opened) as { type?: string };
					if (raw && raw.type === 'pong') {
						onPong(raw as PongMessage);
						return;
					}
					await opts.onMessage(raw);
				} catch (err) {
					console.warn('[ws] message handle failed', err);
				}
			})();
		};

		socket.onerror = () => {
			if (gen !== socketGen) return;
			console.warn('[ws] socket error');
		};

		socket.onclose = (ev) => {
			if (gen !== socketGen) return;
			clearHeartbeat();
			ws = null;
			if (intentionalClose) {
				setStatus('disconnected');
				return;
			}
			console.info('[ws] closed', ev.code, ev.reason || '');
			scheduleReconnect(`close ${ev.code}`);
		};
	}

	function disconnect() {
		intentionalClose = true;
		reconnectGen++;
		clearReconnectTimer();
		clearHeartbeat();
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
		setStatus('disconnected');
	}

	function reconnectNow() {
		intentionalClose = false;
		reconnectAttempt = 0;
		clearReconnectTimer();
		connect({ isReconnect: true });
	}

	function getSocket() {
		return ws;
	}

	function getSocketGen() {
		return socketGen;
	}

	function getStatus() {
		return connectionStatus;
	}

	function getReconnectAttempt() {
		return reconnectAttempt;
	}

	return {
		connect,
		disconnect,
		reconnectNow,
		wsSendJSON,
		wsSendReliable,
		waitForOpenSocket,
		ensureCryptoKey,
		getSocket,
		getSocketGen,
		getStatus,
		getReconnectAttempt,
		/** For handlers that need to ignore stale sockets. */
		isCurrentSocket(socket: WebSocket, gen: number) {
			return gen === socketGen && ws === socket;
		}
	};
}

export type WsSession = ReturnType<typeof createWsSession>;
