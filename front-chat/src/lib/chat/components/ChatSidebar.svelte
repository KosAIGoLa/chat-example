<script lang="ts">
	import type { ChatMode, OnlineUser } from '../types';
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

	interface Props {
		chatMode: ChatMode;
		targetUser?: string;
		groupId?: string;
		joinedGroups: string[];
		/** Global online users — private DM list only. */
		onlineUsers: OnlineUser[];
		myUserId: string;
		unreadPeers?: Record<string, boolean>;
		onModeChange: (mode: ChatMode) => void;
		onJoinGroup: () => void;
		onLeaveGroup: (g: string) => void;
		onSelectGroup: (g: string) => void;
		/** Open private chat with user (user_id, optional username). */
		onSelectUser: (userId: string, username?: string) => void;
		onRefreshOnline: () => void;
	}

	let {
		chatMode,
		targetUser = $bindable(''),
		groupId = $bindable(''),
		joinedGroups,
		onlineUsers,
		myUserId,
		unreadPeers = {},
		onModeChange,
		onJoinGroup,
		onLeaveGroup,
		onSelectGroup,
		onSelectUser,
		onRefreshOnline
	}: Props = $props();

	/** Join form field — separate from the active group id so typing does not switch chat. */
	let joinGroupInput = $state('');

	// Never show yourself in the private list.
	const others = $derived(onlineUsers.filter((u) => u.user_id !== myUserId));

	function handleJoin() {
		const g = joinGroupInput.trim();
		if (!g) return;
		groupId = g;
		onJoinGroup();
	}
</script>

<aside class="bg-sidebar text-sidebar-foreground flex w-64 shrink-0 flex-col border-r">
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
					Direct message
				</p>
				<Input
					bind:value={targetUser}
					placeholder="User ID or pick below"
					onkeydown={(e) => {
						if (e.key === 'Enter' && targetUser.trim()) {
							e.preventDefault();
							onSelectUser(targetUser.trim());
						}
					}}
					onblur={() => {
						if (targetUser.trim()) onSelectUser(targetUser.trim());
					}}
				/>
			</div>
		{:else}
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
		{/if}
	</div>

	<Separator />

	{#if chatMode === 'private'}
		<div class="flex min-h-0 flex-1 flex-col">
			<div class="flex items-center justify-between px-4 py-3">
				<p class="text-muted-foreground text-xs font-medium tracking-wide uppercase">
					Online · {others.length}
				</p>
				<Button variant="ghost" size="icon-xs" onclick={onRefreshOnline} aria-label="Refresh">
					<RefreshCw class="size-3.5" />
				</Button>
			</div>
			<ScrollArea.Root class="min-h-0 flex-1 px-2 pb-4">
				<ul class="space-y-0.5 px-2">
					{#each others as u (u.user_id)}
						<li>
							<button
								type="button"
								class="hover:bg-sidebar-accent flex w-full items-center gap-2.5 rounded-md px-2 py-2 text-left text-sm transition-colors
									{targetUser === u.user_id ? 'bg-sidebar-accent' : ''}
									{unreadPeers[u.user_id]
										? 'bg-amber-500/15 ring-1 ring-amber-400/50 animate-pulse'
										: ''}"
								onclick={() => onSelectUser(u.user_id, u.username)}
								title={unreadPeers[u.user_id]
									? `Unread · open chat with ${u.username || u.user_id}`
									: `id: ${u.user_id}`}
							>
								<span class="relative flex size-2.5 shrink-0">
									{#if unreadPeers[u.user_id]}
										<!-- Unread only: amber flash. Click opens chat → goes normal. -->
										<span
											class="absolute inline-flex size-full animate-ping rounded-full bg-amber-400 opacity-80"
										></span>
										<span class="relative inline-flex size-2.5 rounded-full bg-amber-500"></span>
									{:else}
										<!-- Online: steady green, no ping (not confused with unread). -->
										<span class="relative inline-flex size-2.5 rounded-full bg-emerald-500"></span>
									{/if}
								</span>
								<span class="min-w-0 flex-1 truncate font-medium">{u.username || u.user_id}</span>
								{#if unreadPeers[u.user_id]}
									<span
										class="bg-amber-500 size-2 shrink-0 animate-pulse rounded-full"
										aria-label="Unread message"
									></span>
								{/if}
							</button>
						</li>
					{:else}
						<li class="text-muted-foreground px-2 py-6 text-center text-sm">No users online</li>
					{/each}
				</ul>
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
							>
								<Hash class="text-muted-foreground size-3.5 shrink-0" />
								<span class="truncate font-medium">{g}</span>
								{#if groupId === g}
									<Badge variant="secondary" class="ml-auto text-[10px]">active</Badge>
								{/if}
							</button>
							<Button
								variant="ghost"
								size="icon-xs"
								class="text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100"
								onclick={() => onLeaveGroup(g)}
								aria-label="Leave {g}"
							>
								<LogOut class="size-3.5" />
							</Button>
						</li>
					{:else}
						<li class="text-muted-foreground px-2 py-6 text-center text-sm">
							Join a group to start
						</li>
					{/each}
				</ul>
			</ScrollArea.Root>
		</div>
	{/if}
</aside>
