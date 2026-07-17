import {
	isEncryptedContent,
	tryDecryptContent
} from '../crypto';
import type { ChatMessage, ChatMode } from '../types';

export function isChatContent(msg: ChatMessage): boolean {
	if (msg.type !== 'private' && msg.type !== 'group') return false;
	// Recalled messages keep their slot in history with empty body.
	if (msg.recalled) return true;
	if (msg.content_type === 'voice') return !!msg.media_url;
	if (msg.content_type === 'red_packet') return !!msg.red_packet_id || !!msg.content;
	// System notices (join/leave) and normal text both need non-empty content.
	return !!msg.content;
}

export async function decryptMessage(msg: ChatMessage): Promise<ChatMessage> {
	if (!msg.content || (!msg.encrypted && !isEncryptedContent(msg.content))) {
		return msg;
	}
	const plain = await tryDecryptContent(msg.content);
	return { ...msg, content: plain, encrypted: false };
}

export async function decryptMessages(list: ChatMessage[]): Promise<ChatMessage[]> {
	return Promise.all(list.map((m) => decryptMessage(m)));
}

export function belongsToConversation(
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

export function messageKey(msg: ChatMessage): string {
	if (msg.id) return `id:${msg.id}`;
	return `${msg.type}|${msg.from}|${msg.to}|${msg.group_id ?? ''}|${msg.content_type ?? 'text'}|${msg.media_url ?? ''}|${msg.content}|${msg.timestamp ?? 0}`;
}

export function newMsgId(): string {
	if (typeof crypto !== 'undefined' && crypto.randomUUID) {
		return crypto.randomUUID().replace(/-/g, '');
	}
	return `${Date.now().toString(16)}${Math.random().toString(16).slice(2, 14)}`
		.padEnd(32, '0')
		.slice(0, 32);
}

/** Conversation key for sidebar previews / cache. */
export function conversationKey(msg: ChatMessage, myUserId: string): string | null {
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

export function filterBlockedMessages(
	list: ChatMessage[],
	myUserId: string,
	blockedIds: Set<string> | string[]
): ChatMessage[] {
	if (!list.length) return list;
	const blocked = blockedIds instanceof Set ? blockedIds : new Set(blockedIds);
	if (!blocked.size) return list;
	return list.filter((m) => {
		if (!m.from || m.from === myUserId) return true;
		if (m.content_type === 'system') return true;
		if (m.type !== 'private' && m.type !== 'group') return true;
		return !blocked.has(m.from);
	});
}
