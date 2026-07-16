import type { ChatMessage } from './types';

export function formatMessageLabel(msg: ChatMessage): string {
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

/** Format seconds as m:ss */
export function formatDuration(seconds: number | undefined): string {
	if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return '0:00';
	const s = Math.round(seconds);
	const m = Math.floor(s / 60);
	const r = s % 60;
	return `${m}:${r.toString().padStart(2, '0')}`;
}
