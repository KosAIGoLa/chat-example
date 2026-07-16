<script lang="ts">
	import type { OnlineUser } from '../types';
	import { Button } from '$lib/components/ui/button';
	import * as ScrollArea from '$lib/components/ui/scroll-area';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import MessageCircle from '@lucide/svelte/icons/message-circle';
	import Users from '@lucide/svelte/icons/users';

	interface Props {
		groupId: string;
		members: OnlineUser[];
		myUserId: string;
		/** user_id → unread private flag */
		unreadPeers?: Record<string, boolean>;
		onRefresh: () => void;
		onSelectUser: (userId: string, username?: string) => void;
	}

	let {
		groupId,
		members,
		myUserId,
		unreadPeers = {},
		onRefresh,
		onSelectUser
	}: Props = $props();

	// Safety: never render yourself even if API sent you.
	const others = $derived(members.filter((u) => u.user_id !== myUserId));
</script>

<aside
	class="bg-sidebar/80 text-sidebar-foreground flex w-56 shrink-0 flex-col border-r"
	aria-label="Group members"
>
	<div class="flex h-12 shrink-0 items-center justify-between gap-2 border-b px-3">
		<div class="min-w-0">
			<p class="text-muted-foreground text-[10px] font-medium tracking-wide uppercase">
				Group users
			</p>
			{#if groupId}
				<p class="truncate text-sm font-medium" title={groupId}>#{groupId}</p>
			{:else}
				<p class="text-muted-foreground truncate text-sm">No group</p>
			{/if}
		</div>
		{#if groupId}
			<Button variant="ghost" size="icon-xs" onclick={onRefresh} aria-label="Refresh members">
				<RefreshCw class="size-3.5" />
			</Button>
		{/if}
	</div>

	<div class="text-muted-foreground flex items-center gap-1.5 px-3 py-2 text-xs">
		<Users class="size-3.5 shrink-0" />
		<span>{groupId ? `${others.length} online` : '—'}</span>
	</div>

	<ScrollArea.Root class="min-h-0 flex-1 px-2 pb-4">
		{#if !groupId}
			<p class="text-muted-foreground px-2 py-8 text-center text-sm">Select a group</p>
		{:else}
			<ul class="space-y-0.5 px-1">
				{#each others as u (u.user_id)}
					<li>
						<button
							type="button"
							class="hover:bg-sidebar-accent flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm transition-colors
								{unreadPeers[u.user_id]
									? 'bg-amber-500/15 ring-1 ring-amber-400/50 animate-pulse'
									: ''}"
							onclick={() => onSelectUser(u.user_id, u.username)}
							title={unreadPeers[u.user_id]
								? `Unread · open private chat with ${u.username || u.user_id}`
								: `Private chat with ${u.username || u.user_id}`}
						>
							<span class="relative flex size-2.5 shrink-0">
								{#if unreadPeers[u.user_id]}
									<!-- Unread only flashes; click → private chat + clear. -->
									<span
										class="absolute inline-flex size-full animate-ping rounded-full bg-amber-400 opacity-80"
									></span>
									<span class="relative inline-flex size-2.5 rounded-full bg-amber-500"></span>
								{:else}
									<span class="relative inline-flex size-2.5 rounded-full bg-emerald-500"></span>
								{/if}
							</span>
							<span class="min-w-0 flex-1 truncate font-medium" title="id: {u.user_id}">
								{u.username || u.user_id}
							</span>
							{#if unreadPeers[u.user_id]}
								<span
									class="bg-amber-500 size-2 shrink-0 animate-pulse rounded-full"
									aria-label="Unread"
								></span>
							{:else}
								<MessageCircle class="text-muted-foreground size-3.5 shrink-0 opacity-60" />
							{/if}
						</button>
					</li>
				{:else}
					<li class="text-muted-foreground px-2 py-8 text-center text-sm">
						No other members online
					</li>
				{/each}
			</ul>
		{/if}
	</ScrollArea.Root>
</aside>
