<script lang="ts">
	import type { GroupAnnouncement } from '../types';
	import { Button } from '$lib/components/ui/button';
	import Megaphone from '@lucide/svelte/icons/megaphone';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import ChevronUp from '@lucide/svelte/icons/chevron-up';
	import X from '@lucide/svelte/icons/x';

	interface Props {
		announcements: GroupAnnouncement[];
		/** Owner/admin may remove. */
		canManage?: boolean;
		onRemove?: (messageId: string) => void;
		onOpenMessage?: (messageId: string) => void;
	}

	let {
		announcements,
		canManage = false,
		onRemove,
		onOpenMessage
	}: Props = $props();

	let expanded = $state(false);

	const top = $derived(announcements[0]);
	const count = $derived(announcements.length);
</script>

{#if count > 0 && top}
	<div
		class="border-amber-500/30 bg-amber-500/10 shrink-0 border-b px-3 py-2 md:px-4"
		role="region"
		aria-label="群公告"
	>
		<div class="flex items-start gap-2">
			<div
				class="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-amber-500/20 text-amber-700 dark:text-amber-300"
			>
				<Megaphone class="size-3.5" />
			</div>
			<div class="min-w-0 flex-1">
				<button
					type="button"
					class="w-full text-left"
					onclick={() => {
						if (count > 1) expanded = !expanded;
						else if (top.message_id) onOpenMessage?.(top.message_id);
					}}
				>
					<p class="text-[11px] font-semibold tracking-wide text-amber-800 uppercase dark:text-amber-200">
						群公告
						{#if count > 1}
							· {count} 条
						{/if}
					</p>
					<p class="mt-0.5 line-clamp-2 text-sm text-amber-950/90 dark:text-amber-50/90">
						{#if top.from_username}
							<span class="font-medium">{top.from_username}：</span>
						{/if}
						{top.content}
					</p>
				</button>
				{#if expanded}
					<ul class="mt-2 max-h-40 space-y-1.5 overflow-y-auto pr-1">
						{#each announcements as a (a.message_id)}
							<li
								class="bg-background/60 flex items-start gap-2 rounded-lg border border-amber-500/20 px-2.5 py-1.5 text-sm"
							>
								<button
									type="button"
									class="min-w-0 flex-1 text-left"
									onclick={() => onOpenMessage?.(a.message_id)}
								>
									{#if a.from_username}
										<span class="text-muted-foreground text-xs">{a.from_username}</span>
									{/if}
									<p class="line-clamp-3 leading-snug">{a.content}</p>
								</button>
								{#if canManage}
									<Button
										variant="ghost"
										size="icon-xs"
										class="text-muted-foreground shrink-0"
										title="取消公告"
										onclick={() => onRemove?.(a.message_id)}
									>
										<X class="size-3.5" />
									</Button>
								{/if}
							</li>
						{/each}
					</ul>
				{/if}
			</div>
			{#if count > 1}
				<Button
					variant="ghost"
					size="icon-xs"
					class="text-amber-800 shrink-0 dark:text-amber-200"
					onclick={() => (expanded = !expanded)}
					aria-label={expanded ? '收起公告' : '展开公告'}
				>
					{#if expanded}
						<ChevronUp class="size-4" />
					{:else}
						<ChevronDown class="size-4" />
					{/if}
				</Button>
			{:else if canManage}
				<Button
					variant="ghost"
					size="icon-xs"
					class="text-muted-foreground shrink-0"
					title="取消公告"
					onclick={() => onRemove?.(top.message_id)}
				>
					<X class="size-3.5" />
				</Button>
			{/if}
		</div>
	</div>
{/if}
