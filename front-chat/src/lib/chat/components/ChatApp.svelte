<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { auth } from '$lib/auth.svelte';
	import { createChatController } from '../chat.svelte';
	import { createCallController } from '../call.svelte';
	import ChatHeader from './ChatHeader.svelte';
	import ChatSidebar from './ChatSidebar.svelte';
	import GroupMembersPanel from './GroupMembersPanel.svelte';
	import GroupSettings from './GroupSettings.svelte';
	import MessageList from './MessageList.svelte';
	import MessageInput from './MessageInput.svelte';
	import CallPanel from './CallPanel.svelte';
	import SendRedPacketDialog from './SendRedPacketDialog.svelte';
	import { groupAvatarUrl } from '$lib/api/group.service';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import Hash from '@lucide/svelte/icons/hash';
	import User from '@lucide/svelte/icons/user';
	import Phone from '@lucide/svelte/icons/phone';
	import Video from '@lucide/svelte/icons/video';
	import Users from '@lucide/svelte/icons/users';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import Crown from '@lucide/svelte/icons/crown';
	import Settings from '@lucide/svelte/icons/settings';
	import PanelLeftClose from '@lucide/svelte/icons/panel-left-close';
	import PanelLeftOpen from '@lucide/svelte/icons/panel-left-open';
	import * as Sheet from '$lib/components/ui/sheet';
	import UserAvatar from './UserAvatar.svelte';
	import { confirmDialog, toastError, toastInfo } from '$lib/ui/notify.svelte';
	import { typingUI } from '../typing-ui.svelte';
	import { replyPreviewOf } from '../utils';
	import type { ChatMessage } from '../types';

	const SIDEBAR_KEY = 'ws_chat_sidebar_open';

	const userId = auth.user ? String(auth.user.id) : '';
	let displayUsername = $state(auth.user?.username ?? '');
	let redPacketOpen = $state(false);
	let membersOpen = $state(false);
	let settingsOpen = $state(false);
	let sidebarOpen = $state(
		typeof localStorage !== 'undefined' ? localStorage.getItem(SIDEBAR_KEY) !== '0' : true
	);

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
		onMeetingEvent: (ev) => call.handleMeetingEvent(ev)
	});

	/** Reactive module store — updates even from async WS handlers. */
	const typingHint = $derived(typingUI.hint);
	/** Mirror controller wallet (also updated by red-packet claims). */
	const balance = $derived(chat.balance);

	// UI-local fields bound to inputs; synced into controller on action
	let targetUser = $state('');
	let groupId = $state('');
	let inputText = $state('');
	let callBusy = $state(false);

	// After leaving a group meeting, refresh open-meeting banner for this group.
	let prevCallPhase = $state(call.phase);
	$effect(() => {
		const phase = call.phase;
		const prev = prevCallPhase;
		prevCallPhase = phase;
		if (
			(prev === 'connected' || prev === 'connecting') &&
			(phase === 'idle' || phase === 'ended') &&
			groupId.trim()
		) {
			void chat.refreshGroupMeeting(groupId.trim());
		}
	});

	// Poll open meeting while viewing a group so "加入会议" appears even if WS was missed.
	$effect(() => {
		const gid = groupId.trim();
		const mode = chat.chatMode;
		if (mode !== 'group' || !gid) return;
		void chat.refreshGroupMeeting(gid);
		const t = setInterval(() => {
			void chat.refreshGroupMeeting(gid);
		}, 4000);
		return () => clearInterval(t);
	});

	const privatePeer = $derived.by(() => {
		const id = targetUser.trim();
		if (!id || chat.chatMode !== 'private') return null;
		const f = chat.friends.find((x) => x.user_id === id);
		// Prefer friends[].online from presence; never invent online from message traffic.
		const online = f ? !!f.online : false;
		return {
			id,
			name: chat.displayName(id) || f?.username || id,
			online
		};
	});

	const activeGroup = $derived.by(() => {
		const id = groupId.trim();
		if (!id || chat.chatMode !== 'group') return null;
		const meta = chat.groupMeta[id];
		const meeting = chat.activeMeetings[id];
		const role = meta?.role;
		// Dissolve / role changes: owner only (never treat admin as owner).
		const isOwner = chat.isGroupOwner(id);
		const isAdmin = role === 'admin' && !isOwner;
		const canManage = isOwner || isAdmin || chat.isGroupManager(id);
		return {
			id,
			name: chat.groupDisplayName(id) || meta?.name || id,
			memberCount: meta?.member_count,
			role,
			isOwner,
			isAdmin,
			canManage,
			meeting,
			avatar: meta?.avatar,
			avatarRev: meta?.avatar_rev ?? 0,
			meta
		};
	});

	function toggleSidebar() {
		sidebarOpen = !sidebarOpen;
		try {
			localStorage.setItem(SIDEBAR_KEY, sidebarOpen ? '1' : '0');
		} catch {
			// ignore
		}
	}

	function openGroupSettings(gid?: string) {
		const id = (gid ?? groupId).trim();
		if (!id) return;
		if (id !== groupId.trim()) {
			selectGroup(id);
		}
		settingsOpen = true;
		void chat.refreshGroupMembers(id);
	}

	const conversationTitle = $derived(
		chat.chatMode === 'private'
			? privatePeer
				? privatePeer.name
				: '选择好友开始聊天'
			: activeGroup
				? activeGroup.name
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

	function onProfileUpdated(name: string, _token?: string) {
		displayUsername = name;
		// Auth store already updated by ChatHeader; reconnect WS with new JWT.
		void _token;
		chat.reconnectNow();
	}

	/** Active open meeting for the selected group (if any). */
	const groupMeeting = $derived(
		groupId.trim() ? chat.activeMeetings[groupId.trim()] : undefined
	);
	const inThisGroupMeeting = $derived(
		call.phase !== 'idle' &&
			call.callType === 'group' &&
			!!groupId.trim() &&
			call.groupId === groupId.trim()
	);

	/** Private 1:1 ring call. */
	async function startCall(
		media: 'audio' | 'video' = 'audio',
		peerId?: string,
		peerName?: string
	) {
		if (callBusy || call.phase !== 'idle') return;
		callBusy = true;
		try {
			const to = (peerId ?? targetUser).trim();
			if (peerId || (chat.chatMode === 'private' && to)) {
				if (peerId && peerId !== targetUser) {
					await selectUser(peerId, peerName);
				}
				await call.startPrivateCall(to, peerName || chat.displayName(to), media);
			} else {
				toastInfo('请先选择好友');
			}
		} catch (err) {
			toastError((err as Error).message || '发起通话失败');
		} finally {
			callBusy = false;
		}
	}

	/**
	 * Group conference (meeting mode): open a new meeting, or join if already open.
	 * Not a private ring-call — members enter freely and can keep chatting.
	 */
	async function startOrJoinMeeting(media: 'audio' | 'video' = 'audio') {
		if (callBusy || call.phase !== 'idle') return;
		const gid = groupId.trim();
		if (!gid) {
			toastInfo('请先选择群');
			return;
		}
		callBusy = true;
		try {
			const existing = chat.activeMeetings[gid];
			if (existing) {
				await call.joinGroupMeeting(gid);
			} else {
				await call.startGroupMeeting(gid, media);
			}
			// Prefer server snapshot; fall back to optimistic mark.
			await chat.refreshGroupMeeting(gid);
			if (!chat.activeMeetings[gid]) {
				chat.setActiveMeeting(gid, {
					group_id: gid,
					room: call.roomName || `grp_${gid}`,
					media: call.mediaMode === 'video' ? 'video' : 'audio',
					started_by: userId,
					started_by_name: displayUsername || userId,
					started_at: Math.floor(Date.now() / 1000),
					participant_count: 1
				});
			}
		} catch (err) {
			toastError((err as Error).message || '会议操作失败');
		} finally {
			callBusy = false;
		}
	}

	async function joinOpenMeeting() {
		if (callBusy || call.phase !== 'idle') return;
		const gid = groupId.trim();
		if (!gid) return;
		callBusy = true;
		try {
			// Prefer join; if server has no meeting yet, start with known media.
			try {
				await call.joinGroupMeeting(gid);
			} catch {
				const media = groupMeeting?.media === 'video' ? 'video' : 'audio';
				await call.startGroupMeeting(gid, media);
			}
			await chat.refreshGroupMeeting(gid);
			if (!chat.activeMeetings[gid]) {
				chat.setActiveMeeting(gid, {
					group_id: gid,
					room: call.roomName || `grp_${gid}`,
					media: call.mediaMode === 'video' ? 'video' : 'audio',
					started_by: userId,
					started_by_name: displayUsername || userId,
					started_at: Math.floor(Date.now() / 1000),
					participant_count: 1
				});
			}
			toastInfo('已加入群会议', '会议');
		} catch (err) {
			toastError((err as Error).message || '加入会议失败');
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
		<!-- Col 1: conversation list (can hide) -->
		{#if sidebarOpen}
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
				activeMeetings={chat.activeMeetings}
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
				onRefreshGroups={() => {
					void chat.refreshMyGroups();
				}}
				onOpenGroupSettings={(gid) => openGroupSettings(gid)}
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
		{/if}

		<main class="bg-muted/20 flex min-w-0 flex-1 flex-col">
			<div
				class="bg-background/90 flex h-14 shrink-0 items-center gap-2.5 border-b px-3 backdrop-blur md:px-4"
			>
				<Button
					variant="ghost"
					size="icon"
					class="size-8 shrink-0"
					onclick={toggleSidebar}
					title={sidebarOpen ? '隐藏左侧列表' : '显示左侧列表'}
					aria-label={sidebarOpen ? '隐藏侧栏' : '显示侧栏'}
				>
					{#if sidebarOpen}
						<PanelLeftClose class="size-4" />
					{:else}
						<PanelLeftOpen class="size-4" />
					{/if}
				</Button>
				{#if chat.chatMode === 'private' && privatePeer}
					<div class="relative shrink-0">
						<UserAvatar
							class="size-9"
							name={privatePeer.name}
							userId={privatePeer.id}
							src={`/api/avatar/${encodeURIComponent(privatePeer.id)}`}
						/>
						<span
							class="border-background absolute right-0 bottom-0 size-2.5 rounded-full border-2
								{privatePeer.online ? 'bg-emerald-500' : 'bg-muted-foreground/40'}"
							title={privatePeer.online ? '在线' : '离线'}
						></span>
					</div>
				{:else if chat.chatMode === 'private'}
					<div class="bg-primary/10 text-primary flex size-9 items-center justify-center rounded-full">
						<User class="size-4" />
					</div>
				{:else if chat.chatMode === 'group' && activeGroup}
					{@const gIcon =
						activeGroup.avatar || activeGroup.avatarRev
							? groupAvatarUrl(activeGroup.id, activeGroup.avatarRev)
							: ''}
					<button
						type="button"
						class="relative shrink-0 rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-ring"
						title="群配置"
						onclick={() => openGroupSettings(activeGroup.id)}
					>
						<div
							class="relative flex size-9 items-center justify-center overflow-hidden rounded-xl text-sm font-semibold text-white shadow-sm"
							style="background: hsl({(() => {
								let h = 0;
								const s = activeGroup.id;
								for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
								return (h % 300) + 20;
							})()} 55% 42%)"
						>
							{#if gIcon}
								<img
									src={gIcon}
									alt=""
									class="absolute inset-0 size-full object-cover"
									onerror={(e) => {
										(e.currentTarget as HTMLImageElement).style.display = 'none';
									}}
								/>
							{/if}
							<span class="relative z-0">{activeGroup.name.slice(0, 1).toUpperCase()}</span>
						</div>
					</button>
				{:else}
					<div class="bg-primary/10 text-primary flex size-9 items-center justify-center rounded-full">
						<Hash class="size-4" />
					</div>
				{/if}
				<div class="min-w-0 flex-1">
					<span class="flex min-w-0 items-center gap-1.5">
						<span class="truncate text-sm font-semibold">{conversationTitle}</span>
						{#if activeGroup?.isOwner}
							<span title="群主"><Crown class="size-3.5 shrink-0 text-amber-500" /></span>
						{:else if activeGroup?.isAdmin}
							<span title="管理者" class="text-sky-600 dark:text-sky-400 text-[11px] font-medium">管</span>
						{/if}
					</span>
					{#if typingHint}
						<span class="block truncate text-[11px] font-normal text-emerald-500">
							{typingHint}
						</span>
					{:else if chat.chatMode === 'private' && privatePeer}
						<span
							class="block truncate text-[11px] font-normal
								{privatePeer.online ? 'text-emerald-600 dark:text-emerald-400' : 'text-muted-foreground'}"
						>
							{privatePeer.online ? '在线' : '离线'}
						</span>
					{:else if activeGroup}
						<span class="text-muted-foreground block truncate text-[11px] font-normal">
							{#if activeGroup.meeting}
								<span class="text-emerald-600 dark:text-emerald-400">
									{activeGroup.meeting.media === 'video' ? '视讯' : '语音'}会议进行中
								</span>
								·
							{/if}
							{#if typeof activeGroup.memberCount === 'number'}
								{activeGroup.memberCount} 位成员
							{:else}
								群 ID: {activeGroup.id}
							{/if}
							· 点头像或「群配置」管理
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
						{#if inThisGroupMeeting}
							<Badge
								variant="default"
								class="h-8 gap-1 border-emerald-500/40 bg-emerald-500/15 font-normal text-emerald-700 dark:text-emerald-300"
							>
								{#if call.mediaMode === 'video'}
									<Video class="size-3.5" />
								{:else}
									<Phone class="size-3.5" />
								{/if}
								会议中
							</Badge>
						{:else if groupMeeting}
							<!-- Meeting already open: primary action is JOIN -->
							<Button
								variant="default"
								size="sm"
								class="h-8 gap-1.5 px-2.5 bg-emerald-600 hover:bg-emerald-600/90"
								disabled={callBusy || call.phase !== 'idle'}
								onclick={() => void joinOpenMeeting()}
								title="加入进行中的群会议"
							>
								{#if groupMeeting.media === 'video'}
									<Video class="size-4" />
									<span class="hidden sm:inline">加入视讯</span>
								{:else}
									<Phone class="size-4" />
									<span class="hidden sm:inline">加入语音</span>
								{/if}
							</Button>
						{:else}
							<!-- No meeting yet: open audio or video conference -->
							<Button
								variant="outline"
								size="sm"
								class="h-8 gap-1.5 px-2.5"
								disabled={callBusy || call.phase !== 'idle'}
								onclick={() => void startOrJoinMeeting('audio')}
								title="开启群语音会议；若已有会议则直接加入"
							>
								<Phone class="size-4" />
								<span class="hidden sm:inline">语音会议</span>
							</Button>
							<Button
								variant="default"
								size="sm"
								class="h-8 gap-1.5 px-2.5"
								disabled={callBusy || call.phase !== 'idle'}
								onclick={() => void startOrJoinMeeting('video')}
								title="开启群视讯会议；若已有会议则直接加入"
							>
								<Video class="size-4" />
								<span class="hidden sm:inline">视讯会议</span>
							</Button>
						{/if}
						<Button
							variant="ghost"
							size="sm"
							class="h-8 gap-1.5 px-2.5"
							onclick={() => {
								membersOpen = true;
								void chat.refreshGroupMembers(groupId.trim());
							}}
							title="群成员清单（角色 · 在线状态）"
						>
							<Users class="size-4" />
							<span class="hidden sm:inline">成员</span>
						</Button>
						<Button
							variant="outline"
							size="sm"
							class="h-8 gap-1.5 px-2.5"
							onclick={() => openGroupSettings(groupId)}
							title="群配置：群名、群图片、角色、解散"
						>
							<Settings class="size-4" />
							<span class="hidden sm:inline">群配置</span>
						</Button>
					{/if}
					{#if (chat.chatMode === 'private' && targetUser.trim()) || (chat.chatMode === 'group' && groupId.trim())}
						<Button
							variant="ghost"
							size="sm"
							class="text-muted-foreground h-8 gap-1 px-2"
							title="清除本机缓存的聊天记录（不影响服务器）"
							onclick={() => {
								void (async () => {
									const hasMsgs = chat.messages.length > 0;
									if (!hasMsgs && !targetUser && !groupId) {
										toastInfo('当前没有可清除的本地记录');
										return;
									}
									const ok = await confirmDialog({
										title: '清除本地历史',
										message:
											'清除当前会话在本机缓存的历史消息？\n（服务器记录不受影响，重新打开会话可再同步）',
										confirmText: '清除',
										danger: true
									});
									if (!ok) return;
									const n = chat.clearLocalHistory();
									toastInfo(
										n > 0 ? '已清除本机会话历史' : '当前会话本地记录已清空',
										'本地历史'
									);
								})();
							}}
						>
							<Trash2 class="size-4" />
							<span class="hidden sm:inline">清本地</span>
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

			{#if chat.chatMode === 'group' && groupId.trim() && groupMeeting}
				<div
					class="flex shrink-0 items-center gap-3 border-b border-emerald-500/30 bg-emerald-500/10 px-4 py-2.5 md:px-6"
				>
					<div
						class="flex size-9 shrink-0 items-center justify-center rounded-full bg-emerald-500/20 text-emerald-700 dark:text-emerald-300"
					>
						{#if groupMeeting.media === 'video'}
							<Video class="size-5" />
						{:else}
							<Phone class="size-5" />
						{/if}
					</div>
					<div class="min-w-0 flex-1">
						<p class="truncate text-sm font-semibold text-emerald-800 dark:text-emerald-200">
							{#if inThisGroupMeeting}
								你已在{groupMeeting.media === 'video' ? '视讯' : '语音'}会议中
							{:else}
								群{groupMeeting.media === 'video' ? '视讯' : '语音'}会议进行中 — 点击加入
							{/if}
						</p>
						<p class="text-muted-foreground truncate text-[11px]">
							{groupMeeting.started_by_name || groupMeeting.started_by || '成员'} 发起 ·
							{groupMeeting.participant_count || 1} 人在会 · 也可继续发文字/语音消息
						</p>
					</div>
					{#if inThisGroupMeeting}
						<Badge variant="secondary" class="shrink-0 font-normal">已加入</Badge>
					{:else}
						<Button
							size="default"
							class="h-9 shrink-0 gap-1.5 bg-emerald-600 px-4 font-medium hover:bg-emerald-600/90"
							disabled={callBusy || call.phase !== 'idle'}
							onclick={() => void joinOpenMeeting()}
						>
							{#if groupMeeting.media === 'video'}
								<Video class="size-4" />
								加入视讯会议
							{:else}
								<Phone class="size-4" />
								加入语音会议
							{/if}
						</Button>
					{/if}
				</div>
			{/if}

			<MessageList
				messages={chat.messages}
				myUserId={chat.myUserId}
				loading={chat.historyLoading}
				loadingOlder={chat.historyLoadingOlder}
				hasMore={chat.historyHasMore}
				canReply={chat.chatMode === 'group' && !!groupId.trim()}
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
				onBalanceChange={() => {
					void chat.refreshBalance();
				}}
				onReply={(msg: ChatMessage) => {
					if (chat.chatMode !== 'group' || !groupId.trim()) return;
					if (!msg.from) return;
					// Can reply to anyone's message (including own, for quote).
					const name =
						msg.from === userId
							? displayUsername || msg.from
							: chat.displayName(msg.from) || msg.from;
					chat.setReplyTarget({
						user_id: msg.from,
						username: name || msg.from,
						msg_id: msg.id,
						preview: replyPreviewOf(msg)
					});
					toastInfo(`回复 @${name || msg.from}`, '回复');
				}}
				onEdit={async (msg, text) => {
					await chat.editMessage(msg, text);
					toastInfo('消息已编辑', '编辑');
				}}
				onLoadOlder={() => chat.loadOlderHistory()}
			/>
			<MessageInput
				chatMode={chat.chatMode}
				{targetUser}
				{groupId}
				bind:value={inputText}
				{typingHint}
				replyTarget={chat.replyTarget}
				onClearReply={() => chat.clearReplyTarget()}
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
			void chat.refreshBalance();
		}}
	/>

	{#if chat.chatMode === 'group'}
		<Sheet.Root bind:open={membersOpen}>
			<Sheet.Content side="right" class="w-full p-0 sm:max-w-sm">
				<div class="flex h-full flex-col">
					<Sheet.Header class="border-b px-4 py-3">
						<Sheet.Title>群成员</Sheet.Title>
						<Sheet.Description>
							#{chat.groupDisplayName(groupId) || groupId} · 右键消息可回复
						</Sheet.Description>
					</Sheet.Header>
					<div class="min-h-0 flex-1 overflow-hidden">
						<GroupMembersPanel
							groupId={groupId}
							members={chat.groupMembers}
							myUserId={chat.myUserId}
							unreadPeers={chat.unreadPeers}
							replyUserId={chat.replyTarget?.user_id ?? ''}
							onRefresh={() => chat.refreshGroupMembers()}
							onSelectUser={(uid, name) => {
								membersOpen = false;
								void selectUser(uid, name);
							}}
							onReplyMember={(uid, name) => {
								if (!uid || uid === userId) return;
								chat.setReplyTarget({
									user_id: uid,
									username: name || chat.displayName(uid) || uid
								});
								membersOpen = false;
								toastInfo(`回复 @${name || chat.displayName(uid) || uid}`, '群回复');
							}}
						/>
					</div>
				</div>
			</Sheet.Content>
		</Sheet.Root>

		<Sheet.Root bind:open={settingsOpen}>
			<Sheet.Content side="right" class="w-full gap-0 p-0 sm:max-w-md">
				<div class="flex h-full flex-col">
					<Sheet.Header class="border-b px-4 py-3">
						<Sheet.Title>群配置</Sheet.Title>
						<Sheet.Description>
							{#if activeGroup?.isOwner}
								编辑群名与群图片、管理角色、解散群（仅群主）
							{:else if activeGroup?.canManage}
								编辑群名与群图片（管理者无权解散群）
							{:else}
								查看群资料；可退出本群
							{/if}
						</Sheet.Description>
					</Sheet.Header>
					<div class="min-h-0 flex-1 overflow-hidden">
						{#if groupId.trim()}
							<GroupSettings
								groupId={groupId.trim()}
								meta={chat.groupMeta[groupId.trim()] ?? null}
								members={chat.groupMembers}
								myUserId={chat.myUserId}
								canManage={!!activeGroup?.canManage}
								isOwner={!!activeGroup?.isOwner}
								onRename={async (name) => {
									await chat.renameGroup(groupId.trim(), name);
									toastInfo('群名已更新', '群配置');
									void chat.refreshMyGroups();
								}}
								onUploadAvatar={async (file) => {
									await chat.uploadGroupAvatar(groupId.trim(), file);
									toastInfo('群图片已更新', '群配置');
								}}
								onSetRole={async (uid, role) => {
									await chat.setMemberRole(groupId.trim(), uid, role);
									toastInfo(
										role === 'admin' ? '已升级为管理者' : '已降为一般成员',
										'群配置'
									);
								}}
								onDissolve={async () => {
									const gid = groupId.trim();
									if (!chat.isGroupOwner(gid)) {
										toastError('仅群主可以解散群');
										return;
									}
									await chat.dissolveGroup(gid);
									settingsOpen = false;
									groupId = chat.groupId;
									toastInfo('群已解散', '群配置');
								}}
								onLeave={async () => {
									const gid = groupId.trim();
									await chat.leaveGroup(gid);
									settingsOpen = false;
									groupId = chat.groupId;
									toastInfo('已退出群', '群配置');
								}}
								onRefreshMembers={() => {
									void chat.refreshGroupMembers(groupId.trim());
								}}
							/>
						{/if}
					</div>
				</div>
			</Sheet.Content>
		</Sheet.Root>
	{/if}
</div>
