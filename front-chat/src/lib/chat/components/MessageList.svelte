<script lang="ts">
	import type { ChatMessage } from '../types';
	import MessageBubble from './MessageBubble.svelte';
	import * as ScrollArea from '$lib/components/ui/scroll-area';
	import MessagesSquare from '@lucide/svelte/icons/messages-square';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import ChevronUp from '@lucide/svelte/icons/chevron-up';

	interface Props {
		messages: ChatMessage[];
		myUserId: string;
		loading?: boolean;
		/** Fetching older page while scrolling up. */
		loadingOlder?: boolean;
		/** More older history available on server. */
		hasMore?: boolean;
		/** Enable reply action on group messages. */
		canReply?: boolean;
		canAnnounce?: boolean;
		selectMode?: boolean;
		selectedMsgIds?: string[];
		isAnnouncement?: (messageId: string | undefined) => boolean;
		/** Resolve user id → display name for avatars. */
		resolveName?: (userId: string) => string;
		/** Resolve user id → avatar image URL. */
		resolveAvatar?: (userId: string) => string;
		onRecall?: (msg: ChatMessage) => void;
		onResend?: (msg: ChatMessage) => void;
		onBalanceChange?: (balance: number) => void;
		/** Reply to message (group, via context menu). */
		onReply?: (msg: ChatMessage) => void;
		/** Edit own text message. */
		onEdit?: (msg: ChatMessage, newText: string) => Promise<void> | void;
		onSetAnnouncement?: (msg: ChatMessage) => void;
		onUnsetAnnouncement?: (msg: ChatMessage) => void;
		onEnterSelect?: (msg: ChatMessage) => void;
		onToggleSelect?: (msg: ChatMessage) => void;
		/** Scroll near top → load older history. Returns # of newly prepended messages. */
		onLoadOlder?: () => Promise<number>;
	}

	let {
		messages,
		myUserId,
		loading = false,
		loadingOlder = false,
		hasMore = false,
		canReply = false,
		canAnnounce = false,
		selectMode = false,
		selectedMsgIds = [],
		isAnnouncement,
		resolveName,
		resolveAvatar,
		onRecall,
		onResend,
		onBalanceChange,
		onReply,
		onEdit,
		onSetAnnouncement,
		onUnsetAnnouncement,
		onEnterSelect,
		onToggleSelect,
		onLoadOlder
	}: Props = $props();

	let bottomEl: HTMLDivElement | undefined = $state();
	let topSentinelEl: HTMLDivElement | undefined = $state();
	let bottomSentinelEl: HTMLDivElement | undefined = $state();
	let viewportEl: HTMLElement | null = $state(null);

	/** Stick to latest messages (mobile chat default). */
	let stickToBottom = $state(true);
	/** New messages arrived while user was reading history. */
	let pendingNewCount = $state(0);
	/** Guard concurrent older loads. */
	let loadInFlight = false;

	let prevMsgCount = 0;
	let prevTailId = '';

	const NEAR_BOTTOM_PX = 100;
	const NEAR_TOP_PX = 100;

	function nearBottom(el: HTMLElement, threshold = NEAR_BOTTOM_PX): boolean {
		return el.scrollHeight - el.scrollTop - el.clientHeight < threshold;
	}

	function nearTop(el: HTMLElement, threshold = NEAR_TOP_PX): boolean {
		return el.scrollTop < threshold;
	}

	function scrollToBottom(behavior: ScrollBehavior = 'smooth') {
		const el = viewportEl;
		if (!el) {
			bottomEl?.scrollIntoView({ behavior, block: 'end' });
			return;
		}
		if (behavior === 'smooth') {
			el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
		} else {
			el.scrollTop = el.scrollHeight;
		}
		stickToBottom = true;
		pendingNewCount = 0;
	}

	function jumpToLatest() {
		scrollToBottom('smooth');
	}

	async function tryLoadOlder() {
		if (!onLoadOlder || loadingOlder || loading || !hasMore || loadInFlight) return;
		const el = viewportEl;
		if (!el) return;

		loadInFlight = true;
		const prevHeight = el.scrollHeight;
		const prevTop = el.scrollTop;
		// While prepending older pages, do not treat growth as "new messages".
		const wasStick = stickToBottom;
		stickToBottom = false;

		try {
			const added = await onLoadOlder();
			if (added > 0) {
				// Keep the same messages in view after prepending (mobile chat).
				requestAnimationFrame(() => {
					const next = viewportEl;
					if (!next) return;
					next.scrollTop = prevTop + (next.scrollHeight - prevHeight);
					// If we were filling an undersized list, restore stick after.
					if (wasStick && next.scrollHeight <= next.clientHeight + 8) {
						stickToBottom = true;
						next.scrollTop = next.scrollHeight;
					}
				});
			} else if (wasStick) {
				stickToBottom = true;
			}
		} finally {
			loadInFlight = false;
		}
	}

	function onViewportScroll() {
		const el = viewportEl;
		if (!el) return;
		const atBottom = nearBottom(el);
		stickToBottom = atBottom;
		if (atBottom) pendingNewCount = 0;
		// Pull toward history (top) → load older pages.
		if (nearTop(el)) {
			void tryLoadOlder();
		}
	}

	/**
	 * When messages grow:
	 * - first paint / stickToBottom → snap to bottom (phone chat)
	 * - user reading history → count pending + show "新消息" chip
	 * - older prepend → ignore (handled in tryLoadOlder)
	 */
	$effect(() => {
		const list = messages;
		const len = list.length;
		const isLoading = loading;
		const el = viewportEl;

		const tail = len > 0 ? list[len - 1] : null;
		const tailId = tail
			? `${tail.id ?? ''}:${tail.timestamp ?? 0}:${tail.seq ?? 0}:${tail.content ?? ''}`
			: '';
		const grew = len > prevMsgCount;
		const firstPaint = prevMsgCount === 0 && len > 0;
		const tailChanged = !firstPaint && grew && tailId !== prevTailId && prevTailId !== '';
		// Prepend older: length grows but last message id stays the same.
		const likelyPrepend = grew && !firstPaint && tailId === prevTailId && prevTailId !== '';

		const prevCount = prevMsgCount;
		prevMsgCount = len;
		prevTailId = tailId;

		if (!el || isLoading) return;
		if (!grew && !firstPaint) return;
		if (likelyPrepend || loadingOlder || loadInFlight) return;

		if (stickToBottom || firstPaint) {
			pendingNewCount = 0;
			requestAnimationFrame(() => {
				scrollToBottom(firstPaint ? 'auto' : 'smooth');
			});
			return;
		}

		// User is reading history; new messages arrived at the bottom.
		if (tailChanged || (grew && len > prevCount)) {
			pendingNewCount += Math.max(1, len - prevCount);
		}
	});

	// Scroll listener.
	$effect(() => {
		const el = viewportEl;
		if (!el) return;
		el.addEventListener('scroll', onViewportScroll, { passive: true });
		// Initial position check.
		onViewportScroll();
		return () => el.removeEventListener('scroll', onViewportScroll);
	});

	// Top sentinel: auto-load older when it enters the viewport (more reliable on mobile).
	$effect(() => {
		const root = viewportEl;
		const target = topSentinelEl;
		if (!root || !target || !hasMore || !onLoadOlder) return;

		const io = new IntersectionObserver(
			(entries) => {
				if (entries.some((e) => e.isIntersecting)) {
					void tryLoadOlder();
				}
			},
			{ root, rootMargin: '120px 0px 0px 0px', threshold: 0 }
		);
		io.observe(target);
		return () => io.disconnect();
	});

	// Bottom sentinel: keep stickToBottom accurate without relying only on scroll events.
	$effect(() => {
		const root = viewportEl;
		const target = bottomSentinelEl;
		if (!root || !target) return;

		const io = new IntersectionObserver(
			(entries) => {
				const visible = entries.some((e) => e.isIntersecting);
				if (visible) {
					stickToBottom = true;
					pendingNewCount = 0;
				}
			},
			{ root, rootMargin: '0px 0px 80px 0px', threshold: 0 }
		);
		io.observe(target);
		return () => io.disconnect();
	});

	// If list is shorter than the viewport but more history exists, keep loading
	// until it fills (or hasMore becomes false) — same as many mobile chats.
	$effect(() => {
		const el = viewportEl;
		const len = messages.length;
		if (!el || loading || loadingOlder || loadInFlight || !hasMore || !onLoadOlder) return;
		if (len === 0) return;
		// Need a frame after layout.
		const id = requestAnimationFrame(() => {
			const v = viewportEl;
			if (!v) return;
			if (v.scrollHeight <= v.clientHeight + 24) {
				void tryLoadOlder();
			}
		});
		return () => cancelAnimationFrame(id);
	});

	// Reset pending chip when conversation message set is wiped / switched.
	$effect(() => {
		if (messages.length === 0) {
			pendingNewCount = 0;
			stickToBottom = true;
			prevMsgCount = 0;
			prevTailId = '';
		}
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

	<!-- Mobile-style sticky header while loading older pages -->
	{#if loadingOlder}
		<div
			class="pointer-events-none absolute inset-x-0 top-2 z-10 flex justify-center"
			aria-live="polite"
		>
			<span
				class="bg-card/95 text-muted-foreground inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs shadow-sm backdrop-blur"
			>
				<LoaderCircle class="size-3.5 animate-spin" />
				加载更早的消息…
			</span>
		</div>
	{/if}

	<!-- Jump to latest when user scrolled up and new msgs arrived (phone chat pattern) -->
	{#if pendingNewCount > 0 && !stickToBottom}
		<div class="pointer-events-none absolute inset-x-0 bottom-3 z-10 flex justify-center">
			<button
				type="button"
				class="pointer-events-auto bg-primary text-primary-foreground inline-flex items-center gap-1 rounded-full px-3.5 py-1.5 text-xs font-medium shadow-lg"
				onclick={jumpToLatest}
			>
				<ChevronDown class="size-3.5" />
				{pendingNewCount > 99 ? '99+' : pendingNewCount} 条新消息
			</button>
		</div>
	{:else if !stickToBottom && messages.length > 0}
		<div class="pointer-events-none absolute inset-x-0 bottom-3 z-10 flex justify-center">
			<button
				type="button"
				class="pointer-events-auto bg-card text-muted-foreground hover:text-foreground inline-flex items-center gap-1 rounded-full border px-3 py-1.5 text-xs shadow-md backdrop-blur"
				onclick={jumpToLatest}
				title="回到最新消息"
			>
				<ChevronDown class="size-3.5" />
				回到底部
			</button>
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
							上滑到顶部自动加载更早记录（约 6 个月）
						</p>
					</div>
				</div>
			{:else}
				<!-- Top sentinel: enters view when user scrolls toward older history -->
				<div
					bind:this={topSentinelEl}
					class="flex h-1 w-full shrink-0 flex-col items-center"
					aria-hidden="true"
				></div>

				<div class="flex flex-col items-center gap-1.5 py-1">
					{#if loadingOlder}
						<p class="text-muted-foreground inline-flex items-center gap-1.5 text-xs">
							<LoaderCircle class="size-3.5 animate-spin" />
							加载中…
						</p>
					{:else if hasMore && onLoadOlder}
						<button
							type="button"
							class="text-muted-foreground hover:text-foreground inline-flex h-7 items-center gap-1 rounded-full px-2 text-xs transition-colors"
							onclick={() => void tryLoadOlder()}
						>
							<ChevronUp class="size-3.5" />
							上滑加载更早消息
						</button>
					{:else if messages.length > 0}
						<p class="text-muted-foreground text-[11px]">— 已到最早可同步的记录 —</p>
					{/if}
				</div>

				{#each messages as msg, i (`${msg.id ?? ''}-${msg.timestamp ?? 0}-${msg.from}-${msg.to}-${msg.media_url ?? ''}-${msg.content_type ?? ''}-${i}`)}
					<MessageBubble
						message={msg}
						{myUserId}
						fromName={resolveName?.(msg.from) || msg.from}
						avatarSrc={resolveAvatar?.(msg.from) || ''}
						{canReply}
						{canAnnounce}
						isAnnouncement={!!msg.id && !!isAnnouncement?.(msg.id)}
						{selectMode}
						selected={!!msg.id && selectedMsgIds.includes(msg.id)}
						{onRecall}
						{onResend}
						{onBalanceChange}
						{onReply}
						{onSetAnnouncement}
						{onUnsetAnnouncement}
						{onEnterSelect}
						{onToggleSelect}
						{onEdit}
					/>
				{/each}
			{/if}

			<!-- Bottom anchor + sentinel for "at latest" detection -->
			<div bind:this={bottomEl} class="h-px w-full shrink-0"></div>
			<div bind:this={bottomSentinelEl} class="h-1 w-full shrink-0" aria-hidden="true"></div>
		</div>
	</ScrollArea.Root>
</div>
