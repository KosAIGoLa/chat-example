/**
 * Chat UI package — WebSocket chat components and controller.
 *
 * Layout:
 *   components/     — Svelte UI (ChatApp, bubbles, sidebar, …)
 *   controller/     — domain modules (chat.svelte.ts is thin orchestrator only)
 *     ws-session    — WebSocket connect / heartbeat / sealed send
 *     ws-dispatch   — inbound WS event routing
 *     history       — cache-first history + scroll-up pages
 *     messaging     — send / recall / edit / voice / red packet
 *     typing        — typing indicators
 *     presence      — online / unread / previews
 *     friends       — friends / block (≠ remove)
 *     groups        — groups / roles / announcements
 *     meetings      — group meeting snapshots
 *     message-helpers, normalize, joined-groups, constants
 *   chat.svelte.ts  — orchestrator only (~900 lines, $state + wire)
 *   call.svelte.ts  — LiveKit private call + group meeting
 *
 * Usage:
 *   import { ChatApp } from '$lib/chat';
 *   <ChatApp />
 */
export { default as ChatApp } from './components/ChatApp.svelte';
export { default as ChatHeader } from './components/ChatHeader.svelte';
export { default as ChatSidebar } from './components/ChatSidebar.svelte';
export { default as GroupMembersPanel } from './components/GroupMembersPanel.svelte';
export { default as MessageList } from './components/MessageList.svelte';
export { default as MessageBubble } from './components/MessageBubble.svelte';
export { default as MessageInput } from './components/MessageInput.svelte';
export { default as CallPanel } from './components/CallPanel.svelte';

export { createChatController } from './chat.svelte';
export type { ChatController } from './chat.svelte';
export { createCallController } from './call.svelte';
export type { CallController } from './call.svelte';

export { formatDuration, formatMessageLabel, isOwnMessage, isVoiceMessage } from './utils';
export {
	decryptContent,
	encryptContent,
	importMessageKey,
	isEncryptedContent,
	isWSEncryptedFrame,
	openWSFrame,
	sealWSFrame,
	tryDecryptContent,
	tryOpenWSFrame
} from './crypto';
export type {
	ChatMessage,
	ChatMode,
	ChatUser,
	ConnectionStatus,
	ContentType,
	CryptoKeyResponse,
	GroupMembersEvent,
	GroupMembersResponse,
	OnlineUser,
	PresenceEvent,
	VoiceUploadResult
} from './types';
