<script lang="ts">
	import type {
		BlacklistUser,
		ChatMode,
		FriendRequest,
		FriendUser,
		GroupInfo,
		OnlineUser
	} from '../types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Separator } from '$lib/components/ui/separator';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as ScrollArea from '$lib/components/ui/scroll-area';
	import Users from '@lucide/svelte/icons/users';
	import Hash from '@lucide/svelte/icons/hash';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import UserPlus from '@lucide/svelte/icons/user-plus';
	import LogOut from '@lucide/svelte/icons/log-out';
	import Check from '@lucide/svelte/icons/check';
	import X from '@lucide/svelte/icons/x';
	import UserMinus from '@lucide/svelte/icons/user-minus';
	import Plus from '@lucide/svelte/icons/plus';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import Crown from '@lucide/svelte/icons/crown';
	import Ban from '@lucide/svelte/icons/ban';
	import ShieldOff from '@lucide/svelte/icons/shield-off';
	import Phone from '@lucide/svelte/icons/phone';
	import Video from '@lucide/svelte/icons/video';

	interface Props {
		chatMode: ChatMode;
		targetUser?: string;
		groupId?: string;
		joinedGroups: string[];
		groupMeta?: Record<string, GroupInfo>;
		/** Accepted friends — primary private list. */
		friends: FriendUser[];
		/** Pending invites I received. */
		incomingRequests: FriendRequest[];
		/** Users I blocked. */
		blacklist?: BlacklistUser[];
		/** Global online users (optional browse). */
		onlineUsers: OnlineUser[];
		myUserId: string;
		unreadPeers?: Record<string, boolean>;
		onModeChange: (mode: ChatMode) => void;
		onJoinGroup: () => void;
		onLeaveGroup: (g: string) => void;
		onCreateGroup: (name: string, customId?: string) => Promise<void>;
		onDissolveGroup: (g: string) => Promise<void>;
		onSelectGroup: (g: string) => void;
		onSelectUser: (userId: string, username?: string) => void;
		onRefreshOnline: () => void;
		onRefreshFriends: () => void;
		onInviteFriend: (username: string) => Promise<void>;
		onAcceptRequest: (id: number) => Promise<void>;
		onRejectRequest: (id: number) => Promise<void>;
		onRemoveFriend: (userId: string) => Promise<void>;
		onBlockUser: (opts: { user_id?: string; username?: string }) => Promise<void>;
		onUnblockUser: (userId: string) => Promise<void>;
		/** Start a private LiveKit call with this friend. media: audio | video */
		onCallUser?: (
			userId: string,
			username?: string,
			media?: 'audio' | 'video'
		) => void | Promise<void>;
		callDisabled?: boolean;
	}

	let {
		chatMode,
		targetUser = $bindable(''),
		groupId = $bindable(''),
		joinedGroups,
		groupMeta = {},
		friends,
		incomingRequests,
		blacklist = [],
		onlineUsers,
		myUserId,
		unreadPeers = {},
		onModeChange,
		onJoinGroup,
		onLeaveGroup,
		onCreateGroup,
		onDissolveGroup,
		onSelectGroup,
		onSelectUser,
		onRefreshOnline,
		onRefreshFriends,
		onInviteFriend,
		onAcceptRequest,
		onRejectRequest,
		onRemoveFriend,
		onBlockUser,
		onUnblockUser,
		onCallUser,
		callDisabled = false
	}: Props = $props();

	let joinGroupInput = $state('');
	let createName = $state('');
	let createId = $state('');
	let createBusy = $state(false);
	let createError = $state('');
	let inviteUsername = $state('');
	let inviteBusy = $state(false);
	let inviteError = $state('');
	let inviteOk = $state('');

	const friendIds = $derived(new Set(friends.map((f) => f.user_id)));
	const blockedIds = $derived(new Set(blacklist.map((u) => u.user_id)));
	const othersOnline = $derived(
		onlineUsers.filter(
			(u) => u.user_id !== myUserId && !friendIds.has(u.user_id) && !blockedIds.has(u.user_id)
		)
	);

	function handleJoin() {
		const g = joinGroupInput.trim();
		if (!g) return;
		groupId = g;
		onJoinGroup();
	}

	async function handleCreate() {
		createError = '';
		createBusy = true;
		try {
			await onCreateGroup(createName.trim(), createId.trim() || undefined);
			createName = '';
			createId = '';
		} catch (err) {
			createError = (err as Error).message || 'Create failed';
		} finally {
			createBusy = false;
		}
	}

	function groupLabel(id: string): string {
		return groupMeta[id]?.name || id;
	}

	function isOwner(id: string): boolean {
		const m = groupMeta[id];
		return m?.role === 'owner' || m?.owner_user_id === myUserId;
	}

	async function handleInvite() {
		inviteError = '';
		inviteOk = '';
		const name = inviteUsername.trim();
		if (!name) return;
		inviteBusy = true;
		try {
			await onInviteFriend(name);
			inviteOk = `已向 ${name} 发送好友邀请，对方同意后才会成为好友`;
			inviteUsername = '';
		} catch (err) {
			inviteError = (err as Error).message || 'Invite failed';
		} finally {
			inviteBusy = false;
		}
	}
</script>

<aside class="bg-sidebar text-sidebar-foreground flex w-72 shrink-0 flex-col border-r">
	<div class="space-y-3 p-4">
		<Tabs.Root
			value={chatMode}
			onValueChange={(v) => {
				if (v === 'private' || v === 'group') onModeChange(v);
			}}
			class="w-full"
		>
			<Tabs.List class="grid w-full grid-cols-2">
				<Tabs.Trigger value="private" class="gap-1.5">
					<Users class="size-3.5" />
					Private
				</Tabs.Trigger>
				<Tabs.Trigger value="group" class="gap-1.5">
					<Hash class="size-3.5" />
					Group
				</Tabs.Trigger>
			</Tabs.List>
		</Tabs.Root>

		{#if chatMode === 'private'}
			<div class="space-y-1.5">
				<p class="text-muted-foreground text-xs font-medium tracking-wide uppercase">
					Invite friend
				</p>
				<div class="flex gap-2">
					<Input
						bind:value={inviteUsername}
						placeholder="Username"
						class="flex-1"
						onkeydown={(e) => {
							if (e.key === 'Enter') {
								e.preventDefault();
								void handleInvite();
							}
						}}
					/>
					<Button size="sm" disabled={inviteBusy} onclick={() => void handleInvite()}>
						<UserPlus class="size-4" />
					</Button>
				</div>
				{#if inviteError}
					<p class="text-destructive text-xs">{inviteError}</p>
				{/if}
				{#if inviteOk}
					<p class="text-muted-foreground text-xs">{inviteOk}</p>
				{/if}
			</div>
		{:else}
			<div class="space-y-2">
				<div class="space-y-1.5">
					<p class="text-muted-foreground text-xs font-medium tracking-wide uppercase">
						Create group
					</p>
					<Input bind:value={createName} placeholder="Group name" />
					<Input bind:value={createId} placeholder="Custom id (optional)" />
					<Button
						size="sm"
						class="w-full"
						disabled={createBusy}
						onclick={() => void handleCreate()}
					>
						<Plus class="size-4" />
						Create
					</Button>
					{#if createError}
						<p class="text-destructive text-xs">{createError}</p>
					{/if}
				</div>
				<div class="space-y-1.5">
					<p class="text-muted-foreground text-xs font-medium tracking-wide uppercase">
						Join group
					</p>
					<div class="flex gap-2">
						<Input bind:value={joinGroupInput} placeholder="Group ID" class="flex-1" />
						<Button size="sm" onclick={handleJoin}>
							<UserPlus class="size-4" />
							Join
						</Button>
					</div>
				</div>
			</div>
		{/if}
	</div>

	<Separator />

	{#if chatMode === 'private'}
		<div class="flex min-h-0 flex-1 flex-col">
			{#if incomingRequests.length > 0}
				<div class="px-4 pt-3 pb-1">
					<p class="text-muted-foreground text-xs font-medium tracking-wide uppercase">
						Friend requests · {incomingRequests.length}
					</p>
				</div>
				<ul class="space-y-1 px-3 pb-2">
					{#each incomingRequests as req (req.id)}
						<li class="bg-muted/40 flex items-center gap-2 rounded-md px-2 py-1.5 text-sm">
							<span class="min-w-0 flex-1 truncate font-medium">
								{req.from_username || req.from_user_id}
							</span>
							<Button
								variant="ghost"
								size="icon-xs"
								class="text-emerald-600"
								title="Accept"
								onclick={() => void onAcceptRequest(req.id)}
							>
								<Check class="size-3.5" />
							</Button>
							<Button
								variant="ghost"
								size="icon-xs"
								class="text-destructive"
								title="Reject"
								onclick={() => void onRejectRequest(req.id)}
							>
								<X class="size-3.5" />
							</Button>
						</li>
					{/each}
				</ul>
				<Separator />
			{/if}

			<div class="flex items-center justify-between px-4 py-3">
				<p class="text-muted-foreground text-xs font-medium tracking-wide uppercase">
					Friends · {friends.length}
				</p>
				<Button
					variant="ghost"
					size="icon-xs"
					onclick={() => {
						onRefreshFriends();
						onRefreshOnline();
					}}
					aria-label="Refresh"
				>
					<RefreshCw class="size-3.5" />
				</Button>
			</div>
			<ScrollArea.Root class="min-h-0 flex-1 px-2 pb-2">
				<ul class="space-y-0.5 px-2">
					{#each friends as u (u.user_id)}
						<li class="group flex items-center gap-0.5">
							<button
								type="button"
								class="hover:bg-sidebar-accent flex min-w-0 flex-1 items-center gap-2.5 rounded-md px-2 py-2 text-left text-sm transition-colors
									{targetUser === u.user_id ? 'bg-sidebar-accent' : ''}
									{unreadPeers[u.user_id]
										? 'bg-amber-500/15 ring-1 ring-amber-400/50 animate-pulse'
										: ''}"
								onclick={() => onSelectUser(u.user_id, u.username)}
							>
								<span class="relative flex size-2.5 shrink-0">
									{#if unreadPeers[u.user_id]}
										<span
											class="absolute inline-flex size-full animate-ping rounded-full bg-amber-400 opacity-80"
										></span>
										<span class="relative inline-flex size-2.5 rounded-full bg-amber-500"
										></span>
									{:else if u.online}
										<span class="relative inline-flex size-2.5 rounded-full bg-emerald-500"
										></span>
									{:else}
										<span class="bg-muted-foreground/40 relative inline-flex size-2.5 rounded-full"
										></span>
									{/if}
								</span>
								<span class="min-w-0 flex-1 truncate font-medium"
									>{u.username || u.user_id}</span
								>
							</button>
							{#if onCallUser}
								<Button
									variant="ghost"
									size="icon-xs"
									class="text-primary hover:text-primary opacity-80 group-hover:opacity-100"
									title="语音通话"
									disabled={callDisabled}
									onclick={(e) => {
										e.stopPropagation();
										void onCallUser(u.user_id, u.username, 'audio');
									}}
								>
									<Phone class="size-3.5" />
								</Button>
								<Button
									variant="ghost"
									size="icon-xs"
									class="text-primary hover:text-primary opacity-80 group-hover:opacity-100"
									title="视讯通话"
									disabled={callDisabled}
									onclick={(e) => {
										e.stopPropagation();
										void onCallUser(u.user_id, u.username, 'video');
									}}
								>
									<Video class="size-3.5" />
								</Button>
							{/if}
							<Button
								variant="ghost"
								size="icon-xs"
								class="text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100"
								title="解除好友"
								onclick={() => {
									if (confirm(`解除与 ${u.username || u.user_id} 的好友关系？`)) {
										void onRemoveFriend(u.user_id);
									}
								}}
							>
								<UserMinus class="size-3.5" />
							</Button>
							<Button
								variant="ghost"
								size="icon-xs"
								class="text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100"
								title="拉黑"
								onclick={() => {
									if (
										confirm(
											`拉黑 ${u.username || u.user_id}？将解除好友，且无法互相邀请/私聊。`
										)
									) {
										void onBlockUser({ user_id: u.user_id });
									}
								}}
							>
								<Ban class="size-3.5" />
							</Button>
						</li>
					{:else}
						<li class="text-muted-foreground px-2 py-6 text-center text-sm">
							No friends yet — invite someone above
						</li>
					{/each}
				</ul>

				{#if othersOnline.length > 0}
					<div class="px-4 pt-3 pb-1">
						<p class="text-muted-foreground text-xs font-medium tracking-wide uppercase">
							Online (not friends) · {othersOnline.length}
						</p>
						<p class="text-muted-foreground/80 text-[10px]">Invite them to chat privately</p>
					</div>
					<ul class="space-y-0.5 px-2 pb-2">
						{#each othersOnline as u (u.user_id)}
							<li class="group flex items-center gap-0.5">
								<button
									type="button"
									class="hover:bg-sidebar-accent flex min-w-0 flex-1 items-center gap-2.5 rounded-md px-2 py-1.5 text-left text-sm opacity-80"
									onclick={() => {
										inviteUsername = u.username || u.user_id;
									}}
									title="Click to fill invite form"
								>
									<span class="relative inline-flex size-2 shrink-0 rounded-full bg-emerald-500"
									></span>
									<span class="truncate">{u.username || u.user_id}</span>
								</button>
								<Button
									variant="ghost"
									size="icon-xs"
									class="text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100"
									title="拉黑"
									onclick={() => {
										if (confirm(`拉黑 ${u.username || u.user_id}？`)) {
											void onBlockUser({ user_id: u.user_id, username: u.username });
										}
									}}
								>
									<Ban class="size-3.5" />
								</Button>
							</li>
						{/each}
					</ul>
				{/if}

				{#if blacklist.length > 0}
					<div class="px-4 pt-2 pb-1">
						<p class="text-muted-foreground text-xs font-medium tracking-wide uppercase">
							Blacklist · {blacklist.length}
						</p>
					</div>
					<ul class="space-y-0.5 px-2 pb-4">
						{#each blacklist as u (u.user_id)}
							<li class="group flex items-center gap-0.5">
								<span
									class="text-muted-foreground flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-sm"
								>
									<Ban class="size-3.5 shrink-0 opacity-70" />
									<span class="truncate">{u.username || u.user_id}</span>
								</span>
								<Button
									variant="ghost"
									size="icon-xs"
									class="text-muted-foreground hover:text-emerald-600 opacity-0 group-hover:opacity-100"
									title="取消拉黑"
									onclick={() => void onUnblockUser(u.user_id)}
								>
									<ShieldOff class="size-3.5" />
								</Button>
							</li>
						{/each}
					</ul>
				{/if}
			</ScrollArea.Root>
		</div>
	{:else}
		<div class="flex min-h-0 flex-1 flex-col">
			<div class="px-4 py-3">
				<p class="text-muted-foreground text-xs font-medium tracking-wide uppercase">
					My groups · {joinedGroups.length}
				</p>
			</div>
			<ScrollArea.Root class="min-h-0 flex-1 px-2 pb-4">
				<ul class="space-y-0.5 px-2">
					{#each joinedGroups as g (g)}
						<li
							class="hover:bg-sidebar-accent group flex items-center justify-between rounded-md px-2 py-1.5
								{groupId === g ? 'bg-sidebar-accent' : ''}"
						>
							<button
								type="button"
								class="flex min-w-0 flex-1 items-center gap-2 text-left text-sm"
								onclick={() => onSelectGroup(g)}
								title={g}
							>
								<Hash class="text-muted-foreground size-3.5 shrink-0" />
								<span class="min-w-0 flex-1 truncate font-medium">{groupLabel(g)}</span>
								{#if isOwner(g)}
									<Crown class="size-3 shrink-0 text-amber-500" title="Owner" />
								{/if}
								{#if groupId === g}
									<Badge variant="secondary" class="text-[10px]">active</Badge>
								{/if}
							</button>
							{#if isOwner(g)}
								<Button
									variant="ghost"
									size="icon-xs"
									class="text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100"
									onclick={() => {
										if (confirm(`解散群「${groupLabel(g)}」？所有成员将被移除。`)) {
											void onDissolveGroup(g);
										}
									}}
									aria-label="Dissolve {g}"
									title="解散群"
								>
									<Trash2 class="size-3.5" />
								</Button>
							{:else}
								<Button
									variant="ghost"
									size="icon-xs"
									class="text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100"
									onclick={() => onLeaveGroup(g)}
									aria-label="Leave {g}"
									title="退出群"
								>
									<LogOut class="size-3.5" />
								</Button>
							{/if}
						</li>
					{:else}
						<li class="text-muted-foreground px-2 py-6 text-center text-sm">
							Create or join a group
						</li>
					{/each}
				</ul>
			</ScrollArea.Root>
		</div>
	{/if}
</aside>
