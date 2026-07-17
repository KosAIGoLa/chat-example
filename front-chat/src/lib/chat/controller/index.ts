/**
 * Chat controller package — split domain modules.
 * Public entry remains `createChatController` from `../chat.svelte.ts`.
 */

export * from './constants';
export * from './joined-groups';
export * from './message-helpers';
export * from './normalize';
export { createWsSession } from './ws-session';
export type { WsSession, WsSessionOpts } from './ws-session';
export { createHistoryApi } from './history';
export type { HistoryApi, HistoryDeps } from './history';
export { createFriendsApi } from './friends';
export type { FriendsApi, FriendsDeps } from './friends';
export { createGroupsApi } from './groups';
export type { GroupsApi, GroupsDeps } from './groups';
export { createTypingApi } from './typing';
export type { TypingApi, TypingDeps } from './typing';
export { createPresenceApi } from './presence';
export type { PresenceApi, PresenceDeps } from './presence';
export { createMessagingApi } from './messaging';
export type { MessagingApi, MessagingDeps } from './messaging';
export { createMeetingsApi } from './meetings';
export type { MeetingsApi, MeetingsDeps } from './meetings';
export { createWsDispatcher } from './ws-dispatch';
export type { WsDispatcher, WsDispatchDeps } from './ws-dispatch';
export { createChatState } from './create-chat-state.svelte';
export type { ChatState } from './create-chat-state.svelte';
export { wireChatController } from './wire-controller';
export type { WiredChatController, WireControllerOpts } from './wire-controller';
export { createConversationNav } from './conversation-nav';
export type { ConversationNav } from './conversation-nav';
