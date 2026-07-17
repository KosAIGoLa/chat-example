<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { auth } from '$lib/auth.svelte';
	import { createChatController } from '../chat.svelte';
	import { createCallController } from '../call.svelte';
	import ChatHeader from './ChatHeader.svelte';
	import ChatSidebar from './ChatSidebar.svelte';
	import GroupMembersPanel from './GroupMembersPanel.svelte';
	import MessageList from './MessageList.svelte';
	import MessageInput from './MessageInput.svelte';
	import CallPanel from './CallPanel.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import Hash from '@lucide/svelte/icons/hash';
	import User from '@lucide/svelte/icons/user';
	import Phone from '@lucide/svelte/icons/phone';
	import Video from '@lucide/svelte/icons/video';

	const userId = auth.user ? String(auth.user.id) : '';
	let displayUsername = $state(auth.user?.username ?? '');

	const call = createCallController({
		userId,
		username: auth.user?.username || userId
	});

	const chat = createChatController({
		token: auth.token ?? '',
		userId,
		onUnauthorized: () => {
			window.location.href = '/login';
		},
		onCallEvent: (ev) => call.handleCallEvent(ev)
	});

	// UI-local fields bound to inputs; synced into controller on action
	let targetUser = $state('');
	let groupId = $state('');
	let inputText = $state('');
	let callBusy = $state(false);

	const conversationTitle = $derived(
		chat.chatMode === 'private'
			? targetUser
				? `Private · ${chat.displayName(targetUser)}`
				: 'Private chat'
			: groupId
				? `#${chat.groupDisplayName(groupId)}`
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

	/** media: audio = 语音, video = 视讯 */
	async function startCall(
		media: 'audio' | 'video' = 'audio',
		peerId?: string,
		peerName?: string
	) {
		if (callBusy || call.phase !== 'idle') return;
		callBusy = true;
		try {
			// Explicit peer (e.g. friend-list phone icon) or current conversation.
			const to = (peerId ?? targetUser).trim();
			if (peerId || (chat.chatMode === 'private' && to)) {
				if (peerId && peerId !== targetUser) {
					await selectUser(peerId, peerName);
				}
				await call.startPrivateCall(to, peerName || chat.displayName(to), media);
			} else if (chat.chatMode === 'group' && groupId.trim()) {
				await call.startGroupMeeting(groupId.trim(), media);
			} else {
				alert(chat.chatMode === 'private' ? '请先选择好友' : '请先选择群');
			}
		} catch (err) {
			alert((err as Error).message || '发起通话失败');
		} finally {
			callBusy = false;
		}
	}

	onMount(() => {
		if (!auth.isAuthenticated) {
			window.location.href = '/login';
			return;
		}
		chat.connect();
	});

	onDestroy(() => {
		call.dispose();
		chat.disconnect();
	});

	function handleLogout() {
		call.dispose();
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
			groupMeta={chat.groupMeta}
			friends={chat.friends}
			incomingRequests={chat.incomingRequests}
			blacklist={chat.blacklist}
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
			onCreateGroup={async (name, customId) => {
				const g = await chat.createGroup(name, customId);
				groupId = g.id;
			}}
			onDissolveGroup={async (g) => {
				await chat.dissolveGroup(g);
				groupId = chat.groupId;
			}}
			onSelectGroup={selectGroup}
			onSelectUser={selectUser}
			onRefreshOnline={() => chat.refreshOnlineUsers()}
			onRefreshFriends={() => {
				chat.refreshFriends();
				chat.refreshBlacklist();
			}}
			onInviteFriend={async (name) => {
				await chat.inviteFriend(name);
			}}
			onAcceptRequest={async (id) => {
				await chat.acceptFriendRequest(id);
			}}
			onRejectRequest={async (id) => {
				await chat.rejectFriendRequest(id);
			}}
			onRemoveFriend={async (uid) => {
				await chat.removeFriend(uid);
				if (targetUser === uid) targetUser = '';
			}}
			onBlockUser={async (opts) => {
				const entry = await chat.blockUser(opts);
				if (targetUser === entry.user_id) targetUser = '';
			}}
			onUnblockUser={async (uid) => {
				await chat.unblockUser(uid);
			}}
			onCallUser={async (uid, name, media = 'audio') => {
				await startCall(media, uid, name);
			}}
			callDisabled={callBusy || call.phase !== 'idle'}
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
				<span class="min-w-0 flex-1 truncate text-sm font-medium">{conversationTitle}</span>
				<div class="flex shrink-0 items-center gap-1.5">
					{#if chat.chatMode === 'private' && targetUser.trim()}
						<Button
							variant="outline"
							size="sm"
							class="h-8 gap-1.5 px-2.5"
							disabled={callBusy || call.phase !== 'idle'}
							onclick={() => void startCall('audio')}
							title="语音通话（仅麦克风）"
						>
							<Phone class="size-4" />
							<span>语音</span>
						</Button>
						<Button
							variant="default"
							size="sm"
							class="h-8 gap-1.5 px-2.5"
							disabled={callBusy || call.phase !== 'idle'}
							onclick={() => void startCall('video')}
							title="视讯通话（摄像头+麦克风）"
						>
							<Video class="size-4" />
							<span>视讯</span>
						</Button>
					{:else if chat.chatMode === 'group' && groupId.trim()}
						<Button
							variant="outline"
							size="sm"
							class="h-8 gap-1.5 px-2.5"
							disabled={callBusy || call.phase !== 'idle'}
							onclick={() => void startCall('audio')}
							title="群语音会议"
						>
							<Phone class="size-4" />
							<span>语音</span>
						</Button>
						<Button
							variant="default"
							size="sm"
							class="h-8 gap-1.5 px-2.5"
							disabled={callBusy || call.phase !== 'idle'}
							onclick={() => void startCall('video')}
							title="群视讯会议"
						>
							<Video class="size-4" />
							<span>视讯</span>
						</Button>
					{/if}
					{#if chat.historyLoading}
						<Badge variant="secondary" class="font-normal">Loading…</Badge>
					{:else if chat.messages.length > 0}
						<Badge variant="outline" class="text-muted-foreground font-normal">
							{chat.messages.length}
						</Badge>
					{/if}
				</div>
			</div>

			<MessageList
				messages={chat.messages}
				myUserId={chat.myUserId}
				loading={chat.historyLoading}
				onRecall={(msg) => void chat.recallMessage(msg)}
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

	<CallPanel {call} />
</div>
