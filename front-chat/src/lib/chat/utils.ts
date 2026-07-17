import type { ChatMessage } from './types';
import { RECALL_WINDOW_MS } from './types';

export function formatMessageLabel(msg: ChatMessage): string {
	if (msg.recalled) {
		return 'Recalled';
	}
	if (msg.content_type === 'system') {
		return 'System';
	}
	if (msg.type === 'group') {
		return `[Group ${msg.group_id || msg.to}] ${msg.from}`;
	}
	if (msg.type === 'join_group' || msg.type === 'leave_group') {
		return `[System] ${msg.from}`;
	}
	return `[Private] ${msg.from}`;
}

export function isOwnMessage(msg: ChatMessage, myUserId: string): boolean {
	return msg.from === myUserId;
}

export function isVoiceMessage(msg: ChatMessage): boolean {
	return msg.content_type === 'voice' && !!msg.media_url;
}

export function isRedPacketMessage(msg: ChatMessage): boolean {
	return msg.content_type === 'red_packet' && !!msg.red_packet_id;
}

/** Centered roster notice: "Alice 加入到群" / "Alice 退出群". */
export function isSystemMessage(msg: ChatMessage): boolean {
	return msg.content_type === 'system' || msg.type === 'join_group' || msg.type === 'leave_group';
}

/** Short preview for conversation list. */
export function messagePreview(msg: ChatMessage | undefined): string {
	if (!msg) return '';
	if (msg.recalled) return '撤回了一条消息';
	if (isSystemMessage(msg)) return msg.content || '系统消息';
	if (isRedPacketMessage(msg)) return '[红包]';
	if (isVoiceMessage(msg)) return '[语音]';
	return (msg.content || '').slice(0, 40);
}

/** Parse red packet greeting JSON body. */
export function parseRedPacketContent(content: string): {
	greeting: string;
	total_amount?: number;
	total_count?: number;
	packet_type?: string;
} {
	try {
		const o = JSON.parse(content) as Record<string, unknown>;
		return {
			greeting: String(o.greeting ?? content),
			total_amount: typeof o.total_amount === 'number' ? o.total_amount : undefined,
			total_count: typeof o.total_count === 'number' ? o.total_count : undefined,
			packet_type: typeof o.packet_type === 'string' ? o.packet_type : undefined
		};
	} catch {
		return { greeting: content || '恭喜发财' };
	}
}

/** Client-generated message id (32 hex chars). */
export function newMessageId(): string {
	if (typeof crypto !== 'undefined' && crypto.randomUUID) {
		return crypto.randomUUID().replace(/-/g, '');
	}
	return `${Date.now().toString(16)}${Math.random().toString(16).slice(2, 14)}`.padEnd(32, '0').slice(0, 32);
}

/** Whether the sender may still recall this message (client-side hint). */
export function canRecallMessage(msg: ChatMessage, myUserId: string, now = Date.now()): boolean {
	if (!msg.id || msg.recalled) return false;
	if (msg.from !== myUserId) return false;
	if (msg.content_type === 'system' || msg.content_type === 'red_packet') return false;
	if (msg.type !== 'private' && msg.type !== 'group') return false;
	const ts = (msg.timestamp ?? 0) * 1000;
	if (!ts) return false;
	return now - ts <= RECALL_WINDOW_MS;
}

/** Format seconds as m:ss */
export function formatDuration(seconds: number | undefined): string {
	if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return '0:00';
	const s = Math.round(seconds);
	const m = Math.floor(s / 60);
	const r = s % 60;
	return `${m}:${r.toString().padStart(2, '0')}`;
}
