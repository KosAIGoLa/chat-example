/**
 * Message send/receive, recall, edit, voice, red packet, local cache.
 */

import { mediaService } from '$lib/api';
import { redPacketService } from '$lib/api/red-packet.service';
import type { CreateRedPacketBody } from '$lib/api/red-packet.service';
import {
	encryptContent,
	isEncryptedContent,
	tryDecryptContent
} from '../crypto';
import type {
	ChatMessage,
	ChatMode,
	EditEvent,
	ReplyTarget
} from '../types';
import { toastError, toastInfo } from '$lib/ui/notify.svelte';
import {
	convKeyGroup,
	convKeyPrivate,
	maxSeqOf,
	saveConvCache,
	sortMessagesBySeq
} from '../message-cache';
import {
	belongsToConversation,
	decryptMessage,
	isChatContent,
	messageKey,
	newMsgId
} from './message-helpers';

export interface MessagingDeps {
	getMyUserId: () => string;
	getChatMode: () => ChatMode;
	getTargetUser: () => string;
	getGroupId: () => string;
	getMessages: () => ChatMessage[];
	setMessages: (m: ChatMessage[]) => void;
	getInputText: () => string;
	setInputText: (v: string) => void;
	getReplyTarget: () => ReplyTarget | null;
	setReplyTargetState: (t: ReplyTarget | null) => void;
	ensureCryptoKey: () => Promise<void>;
	updatePreview: (msg: ChatMessage) => void;
	isUserBlocked: (uid: string) => boolean;
	removeRemoteTyper: (userId: string) => void;
	ensurePeerListed: (peerId: string, username?: string) => void;
	clearUnread: (peerId: string) => void;
	markUnread: (peerId: string) => void;
	markGroupUnread: (gid: string) => void;
	notifyTypingStop: () => void;
	refreshBalance: () => Promise<void>;
	wsSendReliable: (
		payload: unknown,
		opts?: { attempts?: number; label?: string }
	) => Promise<void>;
}

export function createMessagingApi(deps: MessagingDeps) {
	let persistCacheTimer: ReturnType<typeof setTimeout> | null = null;

	function conversationTarget(): { peer: string; g: string } | null {
		const peer = deps.getTargetUser().trim();
		const g = deps.getGroupId().trim();
		if (deps.getChatMode() === 'private' && !peer) return null;
		if (deps.getChatMode() === 'group' && !g) return null;
		return { peer, g };
	}

	function setReplyTarget(target: ReplyTarget | null) {
		if (!target?.user_id) {
			deps.setReplyTargetState(null);
			return;
		}
		deps.setReplyTargetState({
			user_id: String(target.user_id),
			username: (target.username || target.user_id).trim() || target.user_id,
			msg_id: target.msg_id?.trim() || undefined,
			preview: target.preview?.trim() || undefined
		});
	}

	function clearReplyTarget() {
		deps.setReplyTargetState(null);
	}

	function updateMessageStatus(
		id: string,
		status: NonNullable<ChatMessage['send_status']>,
		patch?: Partial<ChatMessage>
	) {
		deps.setMessages(
			deps.getMessages().map((m) =>
				m.id === id ? { ...m, send_status: status, ...patch } : m
			)
		);
	}

	function activeCacheKey(): string {
		if (deps.getChatMode() === 'private' && deps.getTargetUser().trim()) {
			return convKeyPrivate(deps.getTargetUser().trim());
		}
		if (deps.getChatMode() === 'group' && deps.getGroupId().trim()) {
			return convKeyGroup(deps.getGroupId().trim());
		}
		return '';
	}

	function persistActiveCache(list: ChatMessage[] = deps.getMessages()) {
		const key = activeCacheKey();
		if (!key) return;
		const snapshot = list;
		const maxSeq = maxSeqOf(list);
		if (persistCacheTimer != null) clearTimeout(persistCacheTimer);
		persistCacheTimer = setTimeout(() => {
			persistCacheTimer = null;
			saveConvCache(key, snapshot, maxSeq);
		}, 250);
	}

	function appendMessage(msg: ChatMessage) {
		const myUserId = deps.getMyUserId();
		const messages = deps.getMessages();
		if (msg.id) {
			const idx = messages.findIndex((m) => m.id === msg.id);
			if (idx >= 0) {
				const prev = messages[idx];
				const merged: ChatMessage = {
					...prev,
					...msg,
					seq: msg.seq || prev.seq,
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
				const sorted = sortMessagesBySeq(next);
				deps.setMessages(sorted);
				deps.updatePreview(merged);
				persistActiveCache(sorted);
				return;
			}
		}
		const key = messageKey(msg);
		if (messages.some((m) => messageKey(m) === key)) return;
		const sorted = sortMessagesBySeq([...messages, msg]);
		deps.setMessages(sorted);
		deps.updatePreview(msg);
		persistActiveCache(sorted);
	}

	function applyRecall(id: string) {
		if (!id) return;
		const next = deps.getMessages().map((m) =>
			m.id === id
				? {
						...m,
						recalled: true,
						content: '',
						media_url: undefined,
						encrypted: false,
						edited: false
					}
				: m
		);
		deps.setMessages(next);
		persistActiveCache(next);
	}

	async function applyEditEvent(ev: EditEvent) {
		if (!ev?.id) return;
		let plain = ev.content || '';
		if (ev.encrypted || isEncryptedContent(plain)) {
			try {
				plain = await tryDecryptContent(plain);
			} catch {
				// keep ciphertext if decrypt fails
			}
		}
		const next = deps.getMessages().map((m) =>
			m.id === ev.id
				? {
						...m,
						content: plain,
						encrypted: false,
						edited: true,
						_local_plain: plain
					}
				: m
		);
		deps.setMessages(next);
		persistActiveCache(next);
		const hit = next.find((m) => m.id === ev.id);
		if (hit) deps.updatePreview(hit);
	}

	async function handleIncomingChat(msg: ChatMessage) {
		const myUserId = deps.getMyUserId();
		const chatMode = deps.getChatMode();
		const targetUser = deps.getTargetUser();
		const groupId = deps.getGroupId();

		const plain = await decryptMessage(msg);
		if (
			plain.from &&
			plain.from !== myUserId &&
			deps.isUserBlocked(String(plain.from)) &&
			(plain.type === 'private' || plain.type === 'group')
		) {
			return;
		}
		deps.updatePreview(plain);
		if (plain.from && plain.from !== myUserId) {
			deps.removeRemoteTyper(plain.from);
		}

		if (plain.type === 'private' && plain.from && plain.to === myUserId) {
			const from = String(plain.from);
			if (from === myUserId) return;
			deps.ensurePeerListed(from);
			const viewing = chatMode === 'private' && String(targetUser) === from;
			if (viewing) {
				deps.clearUnread(from);
				appendMessage(plain);
			} else {
				deps.markUnread(from);
			}
			return;
		}

		if (plain.type === 'group' && plain.from !== myUserId) {
			const gid = plain.group_id || plain.to;
			if (gid && !(chatMode === 'group' && String(groupId) === String(gid))) {
				deps.markGroupUnread(String(gid));
				return;
			}
		}

		if (!belongsToConversation(plain, chatMode, myUserId, targetUser, groupId)) {
			return;
		}
		appendMessage(plain);
	}

	async function deliverChatMessage(local: ChatMessage) {
		if (!local.id) return;
		updateMessageStatus(local.id, 'sending');

		try {
			await deps.ensureCryptoKey();
		} catch (err) {
			console.error('[crypto] key load failed', err);
			updateMessageStatus(local.id, 'failed');
			toastError('加密密钥不可用，无法发送');
			return;
		}

		const plain = local._local_plain ?? local.content;
		if (local.content_type === 'red_packet') {
			updateMessageStatus(local.id, 'sent');
			return;
		}
		let cipher: string;
		try {
			cipher = await encryptContent(
				local.content_type === 'voice' ? plain || '🎤 Voice message' : plain
			);
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
			red_packet_id: local.red_packet_id,
			reply_to_user_id: local.reply_to_user_id,
			reply_to_username: local.reply_to_username,
			reply_to_id: local.reply_to_id,
			reply_to_preview: local.reply_to_preview
		};

		try {
			await deps.wsSendReliable(wire, { attempts: 4, label: local.id });
			updateMessageStatus(local.id, 'sent');
		} catch (err) {
			console.error('[ws] deliver failed', err);
			updateMessageStatus(local.id, 'failed');
			toastError((err as Error).message || '发送失败，可点击重试');
		}
	}

	async function sendMessage() {
		if (!deps.getInputText().trim()) return;

		const dest = conversationTarget();
		const chatMode = deps.getChatMode();
		if (!dest) {
			toastInfo(chatMode === 'private' ? '请先选择好友' : '请先选择群聊');
			return;
		}

		const plain = deps.getInputText().trim();
		const id = newMsgId();
		const ts = Math.floor(Date.now() / 1000);
		const replyTarget = deps.getReplyTarget();
		const reply =
			replyTarget?.user_id && (chatMode === 'group' || chatMode === 'private')
				? {
						reply_to_user_id: replyTarget.user_id,
						reply_to_username: replyTarget.username || replyTarget.user_id,
						reply_to_id: replyTarget.msg_id,
						reply_to_preview: replyTarget.preview
					}
				: {};

		const local: ChatMessage = {
			id,
			type: chatMode,
			from: deps.getMyUserId(),
			to: chatMode === 'private' ? dest.peer : dest.g,
			content: plain,
			encrypted: false,
			content_type: 'text',
			group_id: chatMode === 'group' ? dest.g : '',
			timestamp: ts,
			send_status: 'sending',
			_local_plain: plain,
			...reply
		};
		appendMessage(local);
		deps.setInputText('');
		deps.setReplyTargetState(null);
		deps.notifyTypingStop();

		void deliverChatMessage(local);
	}

	async function resendMessage(msg: ChatMessage) {
		if (!msg.id || msg.from !== deps.getMyUserId()) return;
		if (msg.send_status !== 'failed' && msg.send_status !== 'pending') return;
		const local: ChatMessage = {
			...msg,
			send_status: 'sending',
			_local_plain: msg._local_plain ?? msg.content
		};
		deps.setMessages(
			deps.getMessages().map((m) =>
				m.id === msg.id
					? {
							...m,
							send_status: 'sending',
							content: local._local_plain ?? m.content,
							encrypted: false
						}
					: m
			)
		);
		toastInfo('正在重新发送…');
		await deliverChatMessage(local);
	}

	async function flushPendingSends() {
		const pending = deps.getMessages().filter(
			(m) =>
				m.from === deps.getMyUserId() &&
				m.id &&
				(m.send_status === 'failed' || m.send_status === 'pending')
		);
		for (const m of pending) {
			await resendMessage(m);
		}
	}

	async function recallMessage(msg: ChatMessage) {
		if (!msg.id) return;
		if (msg.from !== deps.getMyUserId() || msg.recalled) return;
		if (msg.send_status === 'sending' || msg.send_status === 'failed') {
			toastInfo('消息尚未送达，无法撤回');
			return;
		}
		try {
			await deps.wsSendReliable(
				{
					type: 'recall',
					id: msg.id,
					from: deps.getMyUserId(),
					to: msg.to,
					content: '',
					group_id: msg.group_id ?? ''
				} satisfies ChatMessage,
				{ attempts: 3, label: `recall:${msg.id}` }
			);
			applyRecall(msg.id);
		} catch (err) {
			toastError((err as Error).message || '撤回失败');
		}
	}

	async function editMessage(msg: ChatMessage, newText: string) {
		const id = msg.id;
		const plain = newText.trim();
		if (!id) return;
		if (msg.from !== deps.getMyUserId() || msg.recalled) return;
		if (!plain) {
			toastError('消息内容不能为空');
			return;
		}
		if (msg.content_type && msg.content_type !== 'text') {
			toastError('仅文字消息可编辑');
			return;
		}
		if (
			msg.send_status === 'sending' ||
			msg.send_status === 'failed' ||
			msg.send_status === 'pending'
		) {
			toastInfo('消息尚未送达，无法编辑');
			return;
		}
		try {
			await deps.ensureCryptoKey();
			const cipher = await encryptContent(plain);
			await deps.wsSendReliable(
				{
					type: 'edit',
					id,
					from: deps.getMyUserId(),
					to: msg.to,
					content: cipher,
					encrypted: true,
					group_id: msg.group_id ?? ''
				} satisfies ChatMessage,
				{ attempts: 3, label: `edit:${id}` }
			);
			const next = deps.getMessages().map((m) =>
				m.id === id
					? {
							...m,
							content: plain,
							encrypted: false,
							edited: true,
							_local_plain: plain
						}
					: m
			);
			deps.setMessages(next);
			persistActiveCache(next);
			const hit = next.find((m) => m.id === id);
			if (hit) deps.updatePreview(hit);
		} catch (err) {
			toastError((err as Error).message || '编辑失败');
			throw err;
		}
	}

	async function sendRedPacket(optsSend: {
		total_amount: number;
		total_count?: number;
		greeting?: string;
		type?: 'group' | 'designated';
		target_user_ids?: string[];
	}) {
		const dest = conversationTarget();
		if (!dest) throw new Error('Select a conversation first');
		const chatMode = deps.getChatMode();
		let body: CreateRedPacketBody;
		if (chatMode === 'private') {
			body = {
				type: 'private',
				peer_id: dest.peer,
				total_amount: optsSend.total_amount,
				greeting: optsSend.greeting
			};
		} else if (optsSend.type === 'designated') {
			body = {
				type: 'designated',
				group_id: dest.g,
				target_user_ids: optsSend.target_user_ids ?? [],
				total_amount: optsSend.total_amount,
				greeting: optsSend.greeting
			};
		} else {
			body = {
				type: 'group',
				group_id: dest.g,
				total_amount: optsSend.total_amount,
				total_count: optsSend.total_count ?? 1,
				greeting: optsSend.greeting
			};
		}
		const res = await redPacketService.create(body);
		const msg = res.message as ChatMessage;
		if (msg && isChatContent(msg)) {
			const plain = await decryptMessage(msg);
			if (
				belongsToConversation(
					plain,
					chatMode,
					deps.getMyUserId(),
					deps.getTargetUser(),
					deps.getGroupId()
				)
			) {
				appendMessage(plain);
			} else {
				deps.updatePreview(plain);
			}
		}
		await deps.refreshBalance();
		return res;
	}

	async function sendVoiceMessage(blob: Blob, durationSec: number) {
		const dest = conversationTarget();
		const chatMode = deps.getChatMode();
		if (!dest) {
			throw new Error(chatMode === 'private' ? '请先选择好友' : '请先选择群聊');
		}
		if (blob.size === 0) {
			throw new Error('录音为空');
		}

		const uploaded = await mediaService.uploadVoice(blob, durationSec);
		const plainLabel = '🎤 语音消息';
		const id = newMsgId();
		const replyTarget = deps.getReplyTarget();
		const reply =
			replyTarget?.user_id && (chatMode === 'group' || chatMode === 'private')
				? {
						reply_to_user_id: replyTarget.user_id,
						reply_to_username: replyTarget.username || replyTarget.user_id,
						reply_to_id: replyTarget.msg_id,
						reply_to_preview: replyTarget.preview
					}
				: {};
		const local: ChatMessage = {
			id,
			type: chatMode,
			from: deps.getMyUserId(),
			to: chatMode === 'private' ? dest.peer : dest.g,
			content: plainLabel,
			encrypted: false,
			content_type: 'voice',
			media_url: uploaded.url,
			duration: durationSec > 0 ? durationSec : uploaded.duration,
			group_id: chatMode === 'group' ? dest.g : '',
			timestamp: Math.floor(Date.now() / 1000),
			send_status: 'sending',
			_local_plain: plainLabel,
			...reply
		};
		appendMessage(local);
		deps.setReplyTargetState(null);
		await deliverChatMessage(local);
		if (deps.getMessages().find((m) => m.id === id)?.send_status === 'failed') {
			throw new Error('语音已上传，但发送失败，可点击重试');
		}
	}

	return {
		appendMessage,
		applyRecall,
		applyEditEvent,
		handleIncomingChat,
		sendMessage,
		deliverChatMessage,
		resendMessage,
		flushPendingSends,
		recallMessage,
		editMessage,
		sendVoiceMessage,
		sendRedPacket,
		updateMessageStatus,
		persistActiveCache,
		setReplyTarget,
		clearReplyTarget,
		conversationTarget
	};
}

export type MessagingApi = ReturnType<typeof createMessagingApi>;
