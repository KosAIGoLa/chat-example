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
	import SendRedPacketDialog from './SendRedPacketDialog.svelte';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import Hash from '@lucide/svelte/icons/hash';
	import User from '@lucide/svelte/icons/user';
	import Phone from '@lucide/svelte/icons/phone';
	import Video from '@lucide/svelte/icons/video';
	import Users from '@lucide/svelte/icons/users';
	import * as Sheet from '$lib/components/ui/sheet';
	import { toastError, toastInfo } from '$lib/ui/notify.svelte';
	import { typingUI } from '../typing-ui.svelte';

	const userId = auth.user ? String(auth.user.id) : '';
	let displayUsername = $state(auth.user?.username ?? '');
	let balance = $state(auth.user?.balance ?? 0);
	let redPacketOpen = $state(false);
	let membersOpen = $state(false);

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
		onCallEvent: (ev) => call.handleCallEvent(ev),
		onBalanceChange: (b) => {
			balance = b;
		}
	});

	/** Reactive module store — updates even from async WS handlers. */
	const typingHint = $derived(typingUI.hint);

	// UI-local fields bound to inputs; synced into controller on action
	let targetUser = $state('');
	let groupId = $state('');
	let inputText = $state('');
	let callBusy = $state(false);

	// Keep header balance in sync with controller.
	$effect(() => {
		balance = chat.balance;
	});

	const conversationTitle = $derived(
		chat.chatMode === 'private'
			? targetUser
				? `私聊 · ${chat.displayName(targetUser)}`
				: '选择好友开始聊天'
			: groupId
				? `#${chat.groupDisplayName(groupId)}`
				: '选择或加入群聊'
	);

	function emitTyping() {
		// Group tab + selected group → always group typing (群主/成员都适用).
		const resolvedMode: 'private' | 'group' =
			chat.chatMode === 'group' && groupId.trim()
				? 'group'
				: chat.chatMode === 'private' && targetUser.trim()
					? 'private'
					: groupId.trim()
						? 'group'
						: 'private';

		chat.targetUser = targetUser;
		chat.groupId = groupId;
		chat.notifyTyping({
			mode: resolvedMode,
			peer: targetUser.trim(),
			group: groupId.trim()
		});
	}

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
		// Sending ends local typing advertisement immediately.
		chat.notifyTypingStop({
			mode: chat.chatMode === 'group' && groupId.trim() ? 'group' : 'private',
			peer: targetUser.trim(),
			group: groupId.trim()
		});
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
				toastInfo(chat.chatMode === 'private' ? '请先选择好友' : '请先选择群');
			}
		} catch (err) {
			toastError((err as Error).message || '发起通话失败');
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
		{balance}
		onLogout={handleLogout}
		onReconnect={() => chat.reconnectNow()}
		{onProfileUpdated}
	/>

	<div class="flex min-h-0 flex-1 overflow-hidden">
		<!-- Col 1: conversation list -->
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
			unreadGroups={chat.unreadGroups}
			lastPreviews={chat.lastPreviews}
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

		<main class="bg-muted/20 flex min-w-0 flex-1 flex-col">
			<div
				class="bg-background/90 flex h-14 shrink-0 items-center gap-2 border-b px-4 backdrop-blur md:px-6"
			>
				{#if chat.chatMode === 'private'}
					<div class="bg-primary/10 text-primary flex size-8 items-center justify-center rounded-full">
						<User class="size-4" />
					</div>
				{:else}
					<div class="bg-primary/10 text-primary flex size-8 items-center justify-center rounded-full">
						<Hash class="size-4" />
					</div>
				{/if}
				<div class="min-w-0 flex-1">
					<span class="block truncate text-sm font-semibold">{conversationTitle}</span>
					{#if typingHint}
						<span class="block truncate text-[11px] font-normal text-emerald-500">
							{typingHint}
						</span>
					{/if}
				</div>
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
							<span class="hidden sm:inline">语音</span>
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
							<span class="hidden sm:inline">视讯</span>
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
							<span class="hidden sm:inline">语音</span>
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
							<span class="hidden sm:inline">视讯</span>
						</Button>
						<Button
							variant="ghost"
							size="sm"
							class="h-8 gap-1.5 px-2.5"
							onclick={() => (membersOpen = true)}
							title="群成员"
						>
							<Users class="size-4" />
							<span class="hidden sm:inline">成员</span>
						</Button>
					{/if}
					{#if chat.historyLoading}
						<Badge variant="secondary" class="font-normal">同步中…</Badge>
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
				resolveName={(uid) =>
					uid === chat.myUserId ? displayUsername || uid : chat.displayName(uid)
				}
				resolveAvatar={(uid) => {
					// Self: only when a photo was uploaded (avoid 404 flicker).
					if (uid === chat.myUserId && auth.user) {
						const rev = auth.user.avatar_rev || 0;
						if (auth.user.avatar || rev > 0) {
							return `/api/avatar/${uid}${rev ? `?v=${rev}` : ''}`;
						}
						return '';
					}
					// Peer: probe /api/avatar/:id — UserAvatar shows letters if 404.
					return `/api/avatar/${encodeURIComponent(uid)}`;
				}}
				onRecall={(msg) => void chat.recallMessage(msg)}
				onResend={(msg) => void chat.resendMessage(msg)}
				onBalanceChange={(b) => {
					balance = b;
					void chat.refreshBalance();
				}}
			/>
			<MessageInput
				chatMode={chat.chatMode}
				{targetUser}
				{groupId}
				bind:value={inputText}
				{typingHint}
				onTyping={emitTyping}
				onSend={send}
				onSendVoice={sendVoice}
				onOpenRedPacket={() => {
					if (
						(chat.chatMode === 'private' && targetUser.trim()) ||
						(chat.chatMode === 'group' && groupId.trim())
					) {
						redPacketOpen = true;
					} else {
						toastInfo(chat.chatMode === 'private' ? '请先选择好友' : '请先选择群');
					}
				}}
			/>
		</main>
	</div>

	<CallPanel {call} />

	<SendRedPacketDialog
		open={redPacketOpen}
		chatMode={chat.chatMode}
		{balance}
		members={chat.groupMembers}
		myUserId={chat.myUserId}
		onClose={() => (redPacketOpen = false)}
		onSend={async (opts) => {
			await chat.sendRedPacket(opts);
			balance = chat.balance;
		}}
	/>

	{#if chat.chatMode === 'group'}
		<Sheet.Root bind:open={membersOpen}>
			<Sheet.Content side="right" class="w-full p-0 sm:max-w-sm">
				<div class="flex h-full flex-col">
					<Sheet.Header class="border-b px-4 py-3">
						<Sheet.Title>群成员</Sheet.Title>
						<Sheet.Description>#{chat.groupDisplayName(groupId) || groupId}</Sheet.Description>
					</Sheet.Header>
					<div class="min-h-0 flex-1 overflow-hidden">
						<GroupMembersPanel
							groupId={groupId}
							members={chat.groupMembers}
							myUserId={chat.myUserId}
							unreadPeers={chat.unreadPeers}
							onRefresh={() => chat.refreshGroupMembers()}
							onSelectUser={(uid, name) => {
								membersOpen = false;
								void selectUser(uid, name);
							}}
						/>
					</div>
				</div>
			</Sheet.Content>
		</Sheet.Root>
	{/if}
</div>
