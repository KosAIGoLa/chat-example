<script lang="ts">
	import type { ChatMessage } from '../types';
	import MessageBubble from './MessageBubble.svelte';
	import * as ScrollArea from '$lib/components/ui/scroll-area';
	import { Button } from '$lib/components/ui/button';
	import MessagesSquare from '@lucide/svelte/icons/messages-square';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import ChevronUp from '@lucide/svelte/icons/chevron-up';

	interface Props {
		messages: ChatMessage[];
		myUserId: string;
		loading?: boolean;
		/** Fetching older page while scrolling up. */
		loadingOlder?: boolean;
		/** More older history available on server. */
		hasMore?: boolean;
		/** Resolve user id → display name for avatars. */
		resolveName?: (userId: string) => string;
		/** Resolve user id → avatar image URL. */
		resolveAvatar?: (userId: string) => string;
		onRecall?: (msg: ChatMessage) => void;
		onResend?: (msg: ChatMessage) => void;
		onBalanceChange?: (balance: number) => void;
		/** Scroll to top → load older history. Returns # of new messages. */
		onLoadOlder?: () => Promise<number>;
	}

	let {
		messages,
		myUserId,
		loading = false,
		loadingOlder = false,
		hasMore = false,
		resolveName,
		resolveAvatar,
		onRecall,
		onResend,
		onBalanceChange,
		onLoadOlder
	}: Props = $props();

	let bottomEl: HTMLDivElement | undefined = $state();
	let viewportEl: HTMLElement | null = $state(null);
	let stickToBottom = $state(true);
	let prevMsgCount = 0;

	function nearBottom(el: HTMLElement, threshold = 120): boolean {
		return el.scrollHeight - el.scrollTop - el.clientHeight < threshold;
	}

	function nearTop(el: HTMLElement, threshold = 80): boolean {
		return el.scrollTop < threshold;
	}

	async function tryLoadOlder() {
		if (!onLoadOlder || loadingOlder || loading || !hasMore) return;
		const el = viewportEl;
		if (!el) return;
		const prevHeight = el.scrollHeight;
		const prevTop = el.scrollTop;
		const added = await onLoadOlder();
		if (added > 0) {
			// Keep the same messages in view after prepending.
			requestAnimationFrame(() => {
				const next = viewportEl;
				if (!next) return;
				next.scrollTop = prevTop + (next.scrollHeight - prevHeight);
			});
		}
	}

	function onViewportScroll() {
		const el = viewportEl;
		if (!el) return;
		stickToBottom = nearBottom(el);
		if (nearTop(el)) {
			void tryLoadOlder();
		}
	}

	// Auto-scroll only when user is near bottom and messages grow (new msgs / first load).
	$effect(() => {
		const len = messages.length;
		const isLoading = loading;
		const el = viewportEl;
		const grew = len > prevMsgCount;
		const firstPaint = prevMsgCount === 0 && len > 0;
		prevMsgCount = len;

		if (!el || isLoading || loadingOlder) return;
		if (!grew && !firstPaint) return;
		// Don't jump to bottom when older history was prepended (stickToBottom false).
		if (stickToBottom || firstPaint) {
			requestAnimationFrame(() => {
				bottomEl?.scrollIntoView({ behavior: firstPaint ? 'auto' : 'smooth', block: 'end' });
			});
		}
	});

	// Bind scroll listener on viewport.
	$effect(() => {
		const el = viewportEl;
		if (!el) return;
		el.addEventListener('scroll', onViewportScroll, { passive: true });
		return () => el.removeEventListener('scroll', onViewportScroll);
	});
</script>

<div class="bg-background relative flex min-h-0 flex-1 flex-col">
	{#if loading}
		<div
			class="bg-background/80 text-muted-foreground absolute inset-x-0 top-0 z-10 border-b px-4 py-2 text-center text-xs backdrop-blur"
		>
			正在同步历史消息…
		</div>
	{/if}

	<ScrollArea.Root class="min-h-0 flex-1" bind:viewportRef={viewportEl}>
		<div class="flex flex-col gap-4 px-4 py-6 md:px-6">
			{#if messages.length === 0 && !loading}
				<div
					class="text-muted-foreground flex min-h-[50vh] flex-col items-center justify-center gap-3"
				>
					<div class="bg-muted flex size-12 items-center justify-center rounded-full">
						<MessagesSquare class="size-6 opacity-60" />
					</div>
					<div class="text-center">
						<p class="text-foreground text-sm font-medium">暂无消息</p>
						<p class="text-xs">选择好友或群聊开始会话 · 支持文字、语音与红包</p>
						<p class="text-muted-foreground mt-1 text-[11px]">
							历史可上滑加载（服务器保留约 6 个月）
						</p>
					</div>
				</div>
			{:else}
				<!-- Older history controls -->
				<div class="flex flex-col items-center gap-1.5 py-1">
					{#if loadingOlder}
						<p class="text-muted-foreground inline-flex items-center gap-1.5 text-xs">
							<LoaderCircle class="size-3.5 animate-spin" />
							加载更早的消息…
						</p>
					{:else if hasMore && onLoadOlder}
						<Button
							variant="ghost"
							size="sm"
							class="text-muted-foreground h-7 gap-1 text-xs"
							onclick={() => void tryLoadOlder()}
						>
							<ChevronUp class="size-3.5" />
							加载更早的消息
						</Button>
					{:else if messages.length > 0}
						<p class="text-muted-foreground text-[11px]">已到最早可同步的记录（约 6 个月内）</p>
					{/if}
				</div>

				{#each messages as msg, i (`${msg.id ?? ''}-${msg.timestamp ?? 0}-${msg.from}-${msg.to}-${msg.media_url ?? ''}-${msg.content_type ?? ''}-${i}`)}
					<MessageBubble
						message={msg}
						{myUserId}
						fromName={resolveName?.(msg.from) || msg.from}
						avatarSrc={resolveAvatar?.(msg.from) || ''}
						{onRecall}
						{onResend}
						{onBalanceChange}
					/>
				{/each}
			{/if}
			<div bind:this={bottomEl} class="h-px w-full shrink-0"></div>
		</div>
	</ScrollArea.Root>
</div>
