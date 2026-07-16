/**
 * Chat UI package — WebSocket chat components and controller.
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

export { createChatController } from './chat.svelte';
export type { ChatController } from './chat.svelte';

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
