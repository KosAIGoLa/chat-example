<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { auth } from '$lib/auth.svelte';
	import { createChatController } from '../chat.svelte';
	import ChatHeader from './ChatHeader.svelte';
	import ChatSidebar from './ChatSidebar.svelte';
	import GroupMembersPanel from './GroupMembersPanel.svelte';
	import MessageList from './MessageList.svelte';
	import MessageInput from './MessageInput.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import Hash from '@lucide/svelte/icons/hash';
	import User from '@lucide/svelte/icons/user';

	const userId = auth.user ? String(auth.user.id) : '';
	let displayUsername = $state(auth.user?.username ?? '');

	const chat = createChatController({
		token: auth.token ?? '',
		userId,
		onUnauthorized: () => {
			window.location.href = '/login';
		}
	});

	// UI-local fields bound to inputs; synced into controller on action
	let targetUser = $state('');
	let groupId = $state('');
	let inputText = $state('');

	const conversationTitle = $derived(
		chat.chatMode === 'private'
			? targetUser
				? `Private · ${chat.displayName(targetUser)}`
				: 'Private chat'
			: groupId
				? `#${groupId}`
				: 'Group chat'
	);

	async function selectUser(peerId: string, peerName?: string) {
		if (!peerId || peerId === userId) return;
		await chat.selectPrivateUser(peerId, peerName);
		// Sync UI with controller after mode switch (group → private).
		targetUser = chat.targetUser;
		groupId = chat.groupId;
	}

	function selectGroup(g: string) {
		void chat.selectGroup(g).then(() => {
			groupId = chat.groupId;
		});
		groupId = g;
	}

	async function send() {
		chat.targetUser = targetUser;
		chat.groupId = groupId;
		chat.inputText = inputText;
		await chat.sendMessage();
		inputText = chat.inputText;
	}

	async function sendVoice(blob: Blob, durationSec: number) {
		chat.targetUser = targetUser;
		chat.groupId = groupId;
		await chat.sendVoiceMessage(blob, durationSec);
	}

	async function joinGroup() {
		chat.groupId = groupId;
		await chat.joinGroup();
		groupId = chat.groupId;
	}

	function onProfileUpdated(name: string, _token: string) {
		displayUsername = name;
		// Token already in localStorage; force a clean WS reconnect with the new JWT.
		chat.reconnectNow();
	}

	onMount(() => {
		if (!auth.isAuthenticated) {
			window.location.href = '/login';
			return;
		}
		chat.connect();
	});

	onDestroy(() => {
		chat.disconnect();
	});

	function handleLogout() {
		chat.disconnect();
		auth.logout();
		window.location.href = '/login';
	}
</script>

<div class="bg-background flex h-svh flex-col overflow-hidden">
	<ChatHeader
		username={displayUsername}
		connectionStatus={chat.connectionStatus}
		reconnectAttempt={chat.reconnectAttempt}
		onLogout={handleLogout}
		onReconnect={() => chat.reconnectNow()}
		{onProfileUpdated}
	/>

	<div class="flex min-h-0 flex-1 overflow-hidden">
		<!-- Col 1: Private online users OR groups list -->
		<ChatSidebar
			chatMode={chat.chatMode}
			bind:targetUser
			bind:groupId
			joinedGroups={chat.joinedGroups}
			onlineUsers={chat.onlineUsers}
			myUserId={chat.myUserId}
			unreadPeers={chat.unreadPeers}
			onModeChange={(m) => chat.setChatMode(m)}
			onJoinGroup={joinGroup}
			onLeaveGroup={(g) => {
				void chat.leaveGroup(g).then(() => {
					groupId = chat.groupId;
				});
			}}
			onSelectGroup={selectGroup}
			onSelectUser={selectUser}
			onRefreshOnline={() => chat.refreshOnlineUsers()}
		/>

		<!-- Col 2: group members (only in Group mode) — to the right of groups -->
		{#if chat.chatMode === 'group'}
			<GroupMembersPanel
				groupId={groupId}
				members={chat.groupMembers}
				myUserId={chat.myUserId}
				unreadPeers={chat.unreadPeers}
				onRefresh={() => chat.refreshGroupMembers()}
				onSelectUser={selectUser}
			/>
		{/if}

		<main class="flex min-w-0 flex-1 flex-col">
			<div
				class="bg-background/80 flex h-12 shrink-0 items-center gap-2 border-b px-4 backdrop-blur md:px-6"
			>
				{#if chat.chatMode === 'private'}
					<User class="text-muted-foreground size-4" />
				{:else}
					<Hash class="text-muted-foreground size-4" />
				{/if}
				<span class="truncate text-sm font-medium">{conversationTitle}</span>
				{#if chat.historyLoading}
					<Badge variant="secondary" class="ml-auto font-normal">Loading…</Badge>
				{:else if chat.messages.length > 0}
					<Badge variant="outline" class="text-muted-foreground ml-auto font-normal">
						{chat.messages.length} messages
					</Badge>
				{/if}
			</div>

			<MessageList
				messages={chat.messages}
				myUserId={chat.myUserId}
				loading={chat.historyLoading}
			/>
			<MessageInput
				chatMode={chat.chatMode}
				{targetUser}
				{groupId}
				bind:value={inputText}
				onSend={send}
				onSendVoice={sendVoice}
			/>
		</main>
	</div>
</div>
