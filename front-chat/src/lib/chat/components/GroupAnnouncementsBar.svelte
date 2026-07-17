<script lang="ts">
	import type { GroupAnnouncement } from '../types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import Pin from '@lucide/svelte/icons/pin';
	import X from '@lucide/svelte/icons/x';
	import Maximize2 from '@lucide/svelte/icons/maximize-2';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import Search from '@lucide/svelte/icons/search';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import ChevronUp from '@lucide/svelte/icons/chevron-up';

	interface Props {
		/** Pinned messages (multiple allowed). */
		announcements: GroupAnnouncement[];
		/** Owner/admin may unpin (private: either peer). */
		canManage?: boolean;
		onRemove?: (messageId: string) => void;
		/** Optional: jump to message in timeline. */
		onOpenMessage?: (messageId: string) => void;
	}

	let {
		announcements,
		canManage = false,
		onRemove,
		onOpenMessage
	}: Props = $props();

	/** Current pin index in the carousel. */
	let index = $state(0);
	/** All-pins modal. */
	let listOpen = $state(false);
	/** Search query inside the modal. */
	let query = $state('');
	/** Expanded message ids (show full content). */
	let expandedIds = $state<string[]>([]);

	const count = $derived(announcements.length);
	const current = $derived(count > 0 ? announcements[Math.min(index, count - 1)] : null);

	const filtered = $derived.by(() => {
		const q = query.trim().toLowerCase();
		if (!q) return announcements;
		return announcements.filter((a) => {
			const content = (a.content ?? '').toLowerCase();
			const name = (a.from_username ?? '').toLowerCase();
			const uid = (a.from_user_id ?? '').toLowerCase();
			return content.includes(q) || name.includes(q) || uid.includes(q);
		});
	});

	// Keep index in range when list shrinks / conversation switches.
	$effect(() => {
		const n = announcements.length;
		if (n === 0) {
			index = 0;
			listOpen = false;
			query = '';
			expandedIds = [];
			return;
		}
		if (index >= n) index = n - 1;
	});

	function nextPin() {
		if (count <= 1) return;
		index = (index + 1) % count;
	}

	function openList(e?: Event) {
		e?.stopPropagation();
		if (!current) return;
		query = '';
		expandedIds = current.message_id ? [current.message_id] : [];
		listOpen = true;
	}

	function closeList() {
		listOpen = false;
		query = '';
		expandedIds = [];
	}

	function unpin(messageId: string, e?: Event) {
		e?.stopPropagation();
		if (!messageId) return;
		onRemove?.(messageId);
		expandedIds = expandedIds.filter((id) => id !== messageId);
	}

	function unpinCurrent(e?: Event) {
		e?.stopPropagation();
		if (!current?.message_id) return;
		unpin(current.message_id);
	}

	function toggleExpand(messageId: string) {
		if (expandedIds.includes(messageId)) {
			expandedIds = expandedIds.filter((id) => id !== messageId);
		} else {
			expandedIds = [...expandedIds, messageId];
		}
	}

	function isExpanded(messageId: string): boolean {
		return expandedIds.includes(messageId);
	}

	function formatTime(ts?: number): string {
		if (!ts || ts <= 0) return '';
		const d = new Date(ts > 1e12 ? ts : ts * 1000);
		if (Number.isNaN(d.getTime())) return '';
		return d.toLocaleString();
	}

	function selectInBar(messageId: string) {
		const i = announcements.findIndex((a) => a.message_id === messageId);
		if (i >= 0) index = i;
	}
</script>

{#if count > 0 && current}
	<div
		class="border-amber-500/30 bg-amber-500/10 shrink-0 border-b px-3 py-2 md:px-4"
		role="region"
		aria-label="置顶消息"
	>
		<div class="flex items-start gap-2">
			<div
				class="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-amber-500/20 text-amber-700 dark:text-amber-300"
			>
				<Pin class="size-3.5" />
			</div>

			<!-- Click body → cycle to next pin (when multiple) -->
			<button
				type="button"
				class="min-w-0 flex-1 rounded-md text-left outline-none focus-visible:ring-2 focus-visible:ring-amber-500/40"
				onclick={() => {
					if (count > 1) nextPin();
				}}
				title={count > 1 ? '点击切换下一则置顶' : undefined}
				aria-label={count > 1 ? `置顶 ${index + 1}/${count}，点击切换下一则` : '置顶消息'}
			>
				<div class="flex items-center gap-1.5">
					<p
						class="text-[11px] font-semibold tracking-wide text-amber-800 uppercase dark:text-amber-200"
					>
						置顶
					</p>
					{#if count > 1}
						<span
							class="rounded-full bg-amber-500/20 px-1.5 py-px text-[10px] font-medium tabular-nums text-amber-800 dark:text-amber-200"
						>
							{index + 1}/{count}
						</span>
						<span
							class="text-amber-700/70 dark:text-amber-300/70 flex items-center gap-0.5 text-[10px]"
						>
							下一条
							<ChevronRight class="size-3" />
						</span>
					{/if}
				</div>
				<p class="mt-0.5 line-clamp-2 text-sm text-amber-950/90 dark:text-amber-50/90">
					{#if current.from_username}
						<span class="font-medium">{current.from_username}：</span>
					{/if}
					{current.content}
				</p>
			</button>

			<div class="flex shrink-0 items-center gap-0.5">
				<!-- Open all pins + search modal -->
				<Button
					variant="ghost"
					size="icon-xs"
					class="text-amber-800 dark:text-amber-200"
					title="全部置顶 / 搜索"
					aria-label="查看全部置顶"
					onclick={openList}
				>
					<Maximize2 class="size-3.5" />
				</Button>

				{#if canManage}
					<Button
						variant="ghost"
						size="icon-xs"
						class="text-muted-foreground"
						title="取消置顶"
						aria-label="取消置顶"
						onclick={unpinCurrent}
					>
						<X class="size-3.5" />
					</Button>
				{/if}
			</div>
		</div>

		{#if count > 1}
			<div class="mt-1.5 flex items-center justify-center gap-1" aria-hidden="true">
				{#each announcements as _, i (i)}
					<span
						class="size-1 rounded-full transition-colors {i === index
							? 'bg-amber-600 dark:bg-amber-400'
							: 'bg-amber-500/30'}"
					></span>
				{/each}
			</div>
		{/if}
	</div>

	<!-- All pins modal with search -->
	{#if listOpen}
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<div
			class="bg-background/70 fixed inset-0 z-[110] flex items-end justify-center p-0 backdrop-blur-sm sm:items-center sm:p-4"
			role="presentation"
			onclick={(e) => {
				if (e.target === e.currentTarget) closeList();
			}}
			onkeydown={(e) => {
				if (e.key === 'Escape') {
					e.preventDefault();
					closeList();
				}
			}}
		>
			<div
				class="bg-card flex max-h-[90vh] w-full max-w-lg flex-col overflow-hidden rounded-t-2xl border shadow-2xl sm:max-h-[85vh] sm:rounded-2xl"
				role="dialog"
				aria-modal="true"
				aria-labelledby="pin-list-title"
				tabindex="-1"
			>
				<!-- Header -->
				<div class="flex items-center gap-2 border-b px-4 py-3">
					<div
						class="flex size-8 shrink-0 items-center justify-center rounded-full bg-amber-500/15 text-amber-700 dark:text-amber-300"
					>
						<Pin class="size-4" />
					</div>
					<div class="min-w-0 flex-1">
						<h2 id="pin-list-title" class="text-sm font-semibold">
							全部置顶
							<span class="text-muted-foreground font-normal">· {count} 条</span>
						</h2>
						<p class="text-muted-foreground text-xs">可搜索内容或发送人</p>
					</div>
					<button
						type="button"
						class="text-muted-foreground hover:bg-muted flex size-8 shrink-0 items-center justify-center rounded-md"
						onclick={closeList}
						aria-label="关闭"
					>
						<X class="size-4" />
					</button>
				</div>

				<!-- Search -->
				<div class="border-b px-4 py-2.5">
					<div class="relative">
						<Search
							class="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2"
						/>
						<Input
							type="search"
							class="h-9 pl-9 pr-8"
							placeholder="搜索置顶内容、发送人…"
							bind:value={query}
							autocomplete="off"
						/>
						{#if query}
							<button
								type="button"
								class="text-muted-foreground hover:text-foreground absolute top-1/2 right-2 -translate-y-1/2 rounded p-0.5"
								onclick={() => (query = '')}
								aria-label="清除搜索"
							>
								<X class="size-3.5" />
							</button>
						{/if}
					</div>
					{#if query.trim()}
						<p class="text-muted-foreground mt-1.5 text-xs">
							找到 {filtered.length} / {count} 条
						</p>
					{/if}
				</div>

				<!-- List -->
				<div class="min-h-0 flex-1 overflow-y-auto px-3 py-2">
					{#if filtered.length === 0}
						<div class="text-muted-foreground flex flex-col items-center gap-2 py-12 text-center text-sm">
							<Search class="size-8 opacity-40" />
							<p>没有匹配的置顶消息</p>
							{#if query.trim()}
								<button
									type="button"
									class="text-primary text-xs underline-offset-2 hover:underline"
									onclick={() => (query = '')}
								>
									清除搜索
								</button>
							{/if}
						</div>
					{:else}
						<ul class="space-y-2">
							{#each filtered as a, i (a.message_id)}
								{@const expanded = isExpanded(a.message_id)}
								{@const isCurrent = a.message_id === current.message_id}
								<li
									class="rounded-xl border px-3 py-2.5 transition-colors {isCurrent
										? 'border-amber-500/40 bg-amber-500/10'
										: 'border-border bg-background/50'}"
								>
									<div class="flex items-start gap-2">
										<span
											class="text-muted-foreground mt-0.5 w-5 shrink-0 text-center text-[11px] tabular-nums"
										>
											{i + 1}
										</span>
										<div class="min-w-0 flex-1">
											<div class="flex flex-wrap items-center gap-x-2 gap-y-0.5">
												{#if a.from_username}
													<span class="text-sm font-medium">{a.from_username}</span>
												{/if}
												{#if formatTime(a.message_ts)}
													<span class="text-muted-foreground text-[11px]">
														{formatTime(a.message_ts)}
													</span>
												{/if}
												{#if isCurrent}
													<span
														class="rounded bg-amber-500/20 px-1 py-px text-[10px] font-medium text-amber-800 dark:text-amber-200"
													>
														当前
													</span>
												{/if}
											</div>
											<button
												type="button"
												class="mt-1 w-full text-left text-sm leading-relaxed"
												onclick={() => toggleExpand(a.message_id)}
											>
												{#if expanded}
													<p class="whitespace-pre-wrap break-words">{a.content}</p>
												{:else}
													<p class="line-clamp-2 break-words">{a.content}</p>
												{/if}
											</button>
											<div class="mt-2 flex flex-wrap items-center gap-1">
												<Button
													variant="ghost"
													size="xs"
													class="h-7 gap-1 px-2 text-xs"
													onclick={() => toggleExpand(a.message_id)}
												>
													{#if expanded}
														<ChevronUp class="size-3.5" />
														收起
													{:else}
														<ChevronDown class="size-3.5" />
														展开全文
													{/if}
												</Button>
												{#if count > 1}
													<Button
														variant="ghost"
														size="xs"
														class="h-7 px-2 text-xs"
														onclick={() => {
															selectInBar(a.message_id);
														}}
														title="在顶栏显示此则"
													>
														设为当前
													</Button>
												{/if}
												{#if onOpenMessage && a.message_id}
													<Button
														variant="ghost"
														size="xs"
														class="h-7 px-2 text-xs"
														onclick={() => {
															onOpenMessage?.(a.message_id);
															closeList();
														}}
													>
														定位
													</Button>
												{/if}
												{#if canManage}
													<Button
														variant="ghost"
														size="xs"
														class="text-destructive hover:text-destructive h-7 px-2 text-xs"
														onclick={(e) => unpin(a.message_id, e)}
													>
														取消置顶
													</Button>
												{/if}
											</div>
										</div>
									</div>
								</li>
							{/each}
						</ul>
					{/if}
				</div>

				<div class="flex items-center justify-end gap-2 border-t px-4 py-3">
					<Button variant="default" size="sm" onclick={closeList}>关闭</Button>
				</div>
			</div>
		</div>
	{/if}
{/if}
