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
	const replyPrefix = msg.reply_to_user_id
		? `回复 @${msg.reply_to_username || msg.reply_to_user_id}：`
		: '';
	if (isRedPacketMessage(msg)) return `${replyPrefix}[红包]`;
	if (isVoiceMessage(msg)) return `${replyPrefix}[语音]`;
	const body = (msg.content || '').slice(0, 40);
	return replyPrefix ? `${replyPrefix}${body}` : body;
}

/** Build a short quote snippet for reply_to_preview. */
export function replyPreviewOf(msg: ChatMessage): string {
	if (msg.recalled) return '已撤回的消息';
	if (isSystemMessage(msg)) return (msg.content || '系统消息').slice(0, 80);
	if (isRedPacketMessage(msg)) return '[红包]';
	if (isVoiceMessage(msg)) return `[语音 ${formatDuration(msg.duration)}]`;
	const t = (msg.content || '').replace(/\s+/g, ' ').trim();
	return t.length > 80 ? `${t.slice(0, 80)}…` : t;
}

/** Relative time for conversation list (unix seconds or ms). */
export function formatRelativeTime(ts: number | undefined | null): string {
	if (!ts) return '';
	const ms = ts > 1e12 ? ts : ts * 1000;
	const diff = Date.now() - ms;
	if (diff < 0) return '';
	const sec = Math.floor(diff / 1000);
	if (sec < 60) return '刚刚';
	const min = Math.floor(sec / 60);
	if (min < 60) return `${min}分钟前`;
	const hr = Math.floor(min / 60);
	if (hr < 24) return `${hr}小时前`;
	const day = Math.floor(hr / 24);
	if (day < 7) return `${day}天前`;
	const d = new Date(ms);
	const m = d.getMonth() + 1;
	const dd = d.getDate();
	return `${m}/${dd}`;
}

/** Parse red packet greeting JSON body. */
export function parseRedPacketContent(content: string): {
	greeting: string;
	total_amount?: number;
	total_count?: number;
	packet_type?: string;
	target_user_ids?: string[];
} {
	try {
		const o = JSON.parse(content) as Record<string, unknown>;
		const rawTargets = o.target_user_ids;
		const target_user_ids = Array.isArray(rawTargets)
			? rawTargets.map((x) => String(x)).filter(Boolean)
			: undefined;
		return {
			greeting: String(o.greeting ?? content),
			total_amount: typeof o.total_amount === 'number' ? o.total_amount : undefined,
			total_count: typeof o.total_count === 'number' ? o.total_count : undefined,
			packet_type: typeof o.packet_type === 'string' ? o.packet_type : undefined,
			target_user_ids
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
	if (msg.send_status === 'sending' || msg.send_status === 'failed' || msg.send_status === 'pending') {
		return false;
	}
	const ts = (msg.timestamp ?? 0) * 1000;
	if (!ts) return false;
	return now - ts <= RECALL_WINDOW_MS;
}

/** Whether the sender may still edit this text message (same window as recall). */
export function canEditMessage(msg: ChatMessage, myUserId: string, now = Date.now()): boolean {
	if (!canRecallMessage(msg, myUserId, now)) return false;
	// Text only — voice / red packet cannot be edited.
	const ct = msg.content_type || 'text';
	return ct === 'text';
}

/**
 * Avatar fallback text from username when no photo uploaded.
 * - CJK / mixed: first 1–2 characters (e.g. 张三 → 张三, 王小明 → 王小)
 * - Latin/ids: first 2 letters uppercased (e.g. bcd123 → BC)
 */
export function avatarInitials(name: string, maxChars = 2): string {
	const s = (name || '').trim();
	if (!s) return '?';
	// Prefer grapheme-friendly split
	const chars = Array.from(s);
	// Pure ascii username / id
	if (/^[a-zA-Z0-9_\-.]+$/.test(s)) {
		return s.slice(0, Math.max(1, maxChars)).toUpperCase();
	}
	return chars.slice(0, Math.max(1, maxChars)).join('');
}

/** Format seconds as m:ss */
export function formatDuration(seconds: number | undefined): string {
	if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return '0:00';
	const s = Math.round(seconds);
	const m = Math.floor(s / 60);
	const r = s % 60;
	return `${m}:${r.toString().padStart(2, '0')}`;
}
