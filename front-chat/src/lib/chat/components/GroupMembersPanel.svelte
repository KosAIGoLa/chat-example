<script lang="ts">
	import type { GroupMember } from '../types';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import * as ScrollArea from '$lib/components/ui/scroll-area';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import MessageCircle from '@lucide/svelte/icons/message-circle';
	import Users from '@lucide/svelte/icons/users';
	import Crown from '@lucide/svelte/icons/crown';
	import Shield from '@lucide/svelte/icons/shield';
	import UserAvatar from './UserAvatar.svelte';

	interface Props {
		groupId: string;
		members: GroupMember[];
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

	const onlineCount = $derived(members.filter((m) => m.online).length);
	const totalCount = $derived(members.length);

	function roleLabel(role: string): string {
		if (role === 'owner') return '群主';
		if (role === 'admin') return '管理者';
		return '一般成员';
	}

	function avatarSrc(uid: string): string {
		if (!uid) return '';
		return `/api/avatar/${encodeURIComponent(uid)}`;
	}
</script>

<aside
	class="bg-sidebar/80 text-sidebar-foreground flex h-full min-h-0 w-full flex-col"
	aria-label="群成员"
>
	<div class="flex h-12 shrink-0 items-center justify-between gap-2 border-b px-3">
		<div class="min-w-0">
			<p class="text-muted-foreground text-[10px] font-medium tracking-wide uppercase">
				成员清单
			</p>
			{#if groupId}
				<p class="truncate text-sm font-medium" title={groupId}>#{groupId}</p>
			{:else}
				<p class="text-muted-foreground truncate text-sm">未选择群</p>
			{/if}
		</div>
		{#if groupId}
			<Button variant="ghost" size="icon-xs" onclick={onRefresh} aria-label="刷新成员" title="刷新">
				<RefreshCw class="size-3.5" />
			</Button>
		{/if}
	</div>

	<div class="text-muted-foreground flex items-center justify-between gap-2 px-3 py-2 text-xs">
		<span class="inline-flex items-center gap-1.5">
			<Users class="size-3.5 shrink-0" />
			{groupId ? `${totalCount} 人` : '—'}
		</span>
		{#if groupId}
			<span>
				<span class="text-emerald-600 dark:text-emerald-400">{onlineCount}</span>
				在线 · {Math.max(0, totalCount - onlineCount)} 离线
			</span>
		{/if}
	</div>

	<ScrollArea.Root class="min-h-0 flex-1 px-2 pb-4">
		{#if !groupId}
			<p class="text-muted-foreground px-2 py-8 text-center text-sm">请先选择群聊</p>
		{:else}
			<ul class="space-y-0.5 px-1">
				{#each members as u (u.user_id)}
					{@const isMe = u.user_id === myUserId}
					<li>
						<button
							type="button"
							class="hover:bg-sidebar-accent flex w-full items-center gap-2.5 rounded-lg px-2 py-2 text-left text-sm transition-colors
								{unreadPeers[u.user_id] && !isMe
									? 'bg-amber-500/15 ring-1 ring-amber-400/50'
									: ''}"
							disabled={isMe}
							onclick={() => {
								if (!isMe) onSelectUser(u.user_id, u.username);
							}}
							title={isMe
								? '我'
								: unreadPeers[u.user_id]
									? `未读 · 私聊 ${u.username || u.user_id}`
									: `私聊 ${u.username || u.user_id}`}
						>
							<div class="relative shrink-0">
								<UserAvatar
									class="size-9"
									name={u.username || u.user_id}
									userId={u.user_id}
									src={avatarSrc(u.user_id)}
									alt={u.username}
								/>
								<span
									class="border-background absolute right-0 bottom-0 size-2.5 rounded-full border-2
										{u.online ? 'bg-emerald-500' : 'bg-muted-foreground/40'}"
									title={u.online ? '在线' : '离线'}
								></span>
							</div>

							<div class="min-w-0 flex-1">
								<div class="flex min-w-0 items-center gap-1.5">
									<span class="truncate font-medium" title="id: {u.user_id}">
										{u.username || u.user_id}
										{#if isMe}
											<span class="text-muted-foreground font-normal">(我)</span>
										{/if}
									</span>
									{#if u.role === 'owner'}
										<Badge
											variant="secondary"
											class="h-5 shrink-0 gap-0.5 border border-amber-500/30 bg-amber-500/10 px-1.5 text-[10px] font-medium text-amber-700 dark:text-amber-300"
										>
											<Crown class="size-3" />
											群主
										</Badge>
									{:else if u.role === 'admin'}
										<Badge
											variant="secondary"
											class="h-5 shrink-0 gap-0.5 border border-sky-500/30 bg-sky-500/10 px-1.5 text-[10px] font-medium text-sky-700 dark:text-sky-300"
										>
											<Shield class="size-3" />
											管理者
										</Badge>
									{:else}
										<Badge variant="outline" class="text-muted-foreground h-5 shrink-0 px-1.5 text-[10px] font-normal">
											一般成员
										</Badge>
									{/if}
								</div>
								<p class="text-muted-foreground mt-0.5 text-[11px]">
									{u.online ? '在线' : '离线'}
									· {roleLabel(u.role)}
								</p>
							</div>

							{#if !isMe}
								{#if unreadPeers[u.user_id]}
									<span
										class="bg-amber-500 size-2 shrink-0 animate-pulse rounded-full"
										aria-label="未读"
									></span>
								{:else}
									<MessageCircle class="text-muted-foreground size-3.5 shrink-0 opacity-50" />
								{/if}
							{/if}
						</button>
					</li>
				{:else}
					<li class="text-muted-foreground px-2 py-8 text-center text-sm">暂无成员</li>
				{/each}
			</ul>
		{/if}
	</ScrollArea.Root>
</aside>
