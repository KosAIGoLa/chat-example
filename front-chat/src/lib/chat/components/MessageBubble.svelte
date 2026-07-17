<script lang="ts">
	import { onDestroy } from 'svelte';
	import type { ChatMessage } from '../types';
	import {
		canEditMessage,
		canRecallMessage,
		formatDuration,
		isOwnMessage,
		isRedPacketMessage,
		isSystemMessage,
		isVoiceMessage
	} from '../utils';
	import { mediaService } from '$lib/api/media.service';
	import { cn } from '$lib/utils';
	import { Button } from '$lib/components/ui/button';
	import Mic from '@lucide/svelte/icons/mic';
	import Undo2 from '@lucide/svelte/icons/undo-2';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import Reply from '@lucide/svelte/icons/reply';
	import Pencil from '@lucide/svelte/icons/pencil';
	import RedPacketCard from './RedPacketCard.svelte';
	import UserAvatar from './UserAvatar.svelte';

	interface Props {
		message: ChatMessage;
		myUserId: string;
		/** Optional display name for message.from (username). */
		fromName?: string;
		/** Optional avatar image URL for message.from. */
		avatarSrc?: string;
		/** Allow reply via context menu (group chat). */
		canReply?: boolean;
		onRecall?: (msg: ChatMessage) => void;
		onResend?: (msg: ChatMessage) => void;
		onBalanceChange?: (balance: number) => void;
		/** Reply to this message (quote + @sender). */
		onReply?: (msg: ChatMessage) => void;
		/** Edit own text message. */
		onEdit?: (msg: ChatMessage, newText: string) => Promise<void> | void;
	}

	let {
		message,
		myUserId,
		fromName = '',
		avatarSrc = '',
		canReply = false,
		onRecall,
		onResend,
		onBalanceChange,
		onReply,
		onEdit
	}: Props = $props();

	const own = $derived(isOwnMessage(message, myUserId));
	const voice = $derived(isVoiceMessage(message));
	const system = $derived(isSystemMessage(message));
	const redPacket = $derived(isRedPacketMessage(message));
	const recalled = $derived(!!message.recalled);
	const showRecall = $derived(
		canRecallMessage(message, myUserId) && !!onRecall
	);
	const showEdit = $derived(canEditMessage(message, myUserId) && !!onEdit);
	const showReply = $derived(
		canReply &&
			!!onReply &&
			!system &&
			!recalled &&
			message.type === 'group' &&
			message.send_status !== 'failed'
	);
	const sending = $derived(message.send_status === 'sending' || message.send_status === 'pending');
	const failed = $derived(message.send_status === 'failed');
	const displayLabel = $derived((fromName || message.from || '?').trim() || '?');
	const timeLabel = $derived(
		message.timestamp
			? new Date(message.timestamp * 1000).toLocaleTimeString([], {
					hour: '2-digit',
					minute: '2-digit'
				})
			: ''
	);
	const audioSrc = $derived(
		voice && message.media_url && !recalled ? mediaService.buildMediaUrl(message.media_url) : ''
	);

	const voiceLabel = $derived(`语音 · ${formatDuration(message.duration)}`);
	const hasReplyMeta = $derived(!!message.reply_to_user_id);
	const replyLabel = $derived(message.reply_to_username || message.reply_to_user_id || '');

	// Context menu (right-click)
	let menuOpen = $state(false);
	let menuX = $state(0);
	let menuY = $state(0);

	// Inline edit
	let editing = $state(false);
	let editDraft = $state('');
	let editBusy = $state(false);
	let editTa: HTMLTextAreaElement | undefined = $state();

	$effect(() => {
		if (editing && editTa) {
			editTa.focus();
			editTa.select();
		}
	});

	function closeMenu() {
		menuOpen = false;
	}

	function openMenu(e: MouseEvent) {
		if (system || recalled) return;
		// No actions available → skip menu
		const hasAny =
			showReply ||
			showRecall ||
			showEdit ||
			(failed && own && !!onResend);
		if (!hasAny) return;
		e.preventDefault();
		e.stopPropagation();
		menuX = e.clientX;
		menuY = e.clientY;
		// Keep menu inside viewport
		const pad = 8;
		const w = 180;
		const h = 160;
		if (typeof window !== 'undefined') {
			if (menuX + w > window.innerWidth - pad) menuX = window.innerWidth - w - pad;
			if (menuY + h > window.innerHeight - pad) menuY = window.innerHeight - h - pad;
			if (menuX < pad) menuX = pad;
			if (menuY < pad) menuY = pad;
		}
		menuOpen = true;
	}

	function onDocPointer(e: Event) {
		if (!menuOpen) return;
		const t = e.target as HTMLElement | null;
		if (t?.closest?.('[data-msg-ctx-menu]')) return;
		closeMenu();
	}

	function onKey(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			closeMenu();
			if (editing) cancelEdit();
		}
	}

	$effect(() => {
		if (!menuOpen && !editing) return;
		const opts = { capture: true } as const;
		document.addEventListener('pointerdown', onDocPointer, opts);
		document.addEventListener('keydown', onKey);
		return () => {
			document.removeEventListener('pointerdown', onDocPointer, opts);
			document.removeEventListener('keydown', onKey);
		};
	});

	onDestroy(() => {
		closeMenu();
	});

	function startEdit() {
		closeMenu();
		editDraft = message.content || '';
		editing = true;
	}

	function cancelEdit() {
		editing = false;
		editDraft = '';
		editBusy = false;
	}

	async function saveEdit() {
		const text = editDraft.trim();
		if (!text || !onEdit) return;
		if (text === (message.content || '').trim()) {
			cancelEdit();
			return;
		}
		editBusy = true;
		try {
			await onEdit(message, text);
			editing = false;
			editDraft = '';
		} catch {
			// parent toasts
		} finally {
			editBusy = false;
		}
	}

	function doReply() {
		closeMenu();
		onReply?.(message);
	}

	function doRecall() {
		closeMenu();
		onRecall?.(message);
	}
</script>

{#if system}
	<div class="flex w-full justify-center py-1">
		<div
			class="bg-muted/60 text-muted-foreground inline-flex max-w-[min(28rem,90%)] items-center gap-2 rounded-full px-3 py-1 text-center text-xs"
		>
			<span class="leading-snug">{message.content}</span>
			{#if timeLabel}
				<span class="opacity-60">{timeLabel}</span>
			{/if}
		</div>
	</div>
{:else if recalled}
	<div class="flex w-full justify-center py-1">
		<div
			class="text-muted-foreground inline-flex items-center gap-1.5 rounded-full border border-dashed px-3 py-1 text-center text-xs italic"
		>
			<span>{own ? '你撤回了一条消息' : '对方撤回了一条消息'}</span>
			{#if timeLabel}
				<span class="not-italic opacity-60">{timeLabel}</span>
			{/if}
		</div>
	</div>
{:else}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class={cn('group flex w-full items-end gap-2.5', own ? 'flex-row-reverse' : 'flex-row')}
		oncontextmenu={openMenu}
	>
		<div class="mb-0.5 shrink-0">
			<UserAvatar
				class="size-10"
				name={displayLabel}
				userId={message.from}
				src={avatarSrc}
				primary={own}
				alt={displayLabel}
			/>
		</div>

		<div
			class={cn(
				'flex max-w-[min(32rem,calc(100%-3rem))] flex-col gap-1',
				own ? 'items-end' : 'items-start'
			)}
		>
			<div
				class={cn(
					'text-muted-foreground flex items-center gap-2 px-1 text-[11px]',
					own ? 'flex-row-reverse' : 'flex-row'
				)}
			>
				<span class="max-w-[12rem] truncate font-medium">{displayLabel}</span>
				{#if timeLabel}
					<span class="opacity-70">{timeLabel}</span>
				{/if}
				{#if message.edited && !editing}
					<span class="opacity-60">(已编辑)</span>
				{/if}
			</div>
			<div class="flex items-end gap-1">
				{#if failed && own && onResend}
					<Button
						variant="ghost"
						size="icon-xs"
						class="text-destructive shrink-0"
						title="点击重发"
						onclick={() => onResend?.(message)}
					>
						<RotateCcw class="size-3.5" />
					</Button>
				{/if}
				{#if redPacket}
					<div class={cn(failed && 'opacity-70')}>
						{#if hasReplyMeta}
							<div
								class={cn(
									'mb-1 max-w-[16rem] rounded-lg border-l-2 px-2 py-1 text-[11px]',
									own
										? 'border-primary-foreground/50 bg-primary-foreground/10 text-primary-foreground/90'
										: 'border-primary/50 bg-background/80 text-muted-foreground'
								)}
							>
								<span class="font-semibold">@{replyLabel}</span>
								{#if message.reply_to_preview}
									<span class="mt-0.5 block truncate opacity-90">{message.reply_to_preview}</span>
								{/if}
							</div>
						{/if}
						<RedPacketCard {message} {myUserId} {own} {onBalanceChange} />
					</div>
				{:else if editing}
					<div
						class={cn(
							'flex w-[min(20rem,calc(100vw-6rem))] flex-col gap-2 rounded-2xl border px-3 py-2 shadow-xs',
							own ? 'bg-primary/10 border-primary/30' : 'bg-muted'
						)}
					>
						<textarea
							bind:this={editTa}
							class="bg-background min-h-[4.5rem] w-full resize-y rounded-lg border px-2.5 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
							bind:value={editDraft}
							disabled={editBusy}
							maxlength={2000}
							onkeydown={(e) => {
								if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
									e.preventDefault();
									void saveEdit();
								}
								if (e.key === 'Escape') {
									e.preventDefault();
									cancelEdit();
								}
							}}
						></textarea>
						<div class="flex justify-end gap-1.5">
							<Button
								variant="ghost"
								size="sm"
								class="h-7"
								disabled={editBusy}
								onclick={cancelEdit}
							>
								取消
							</Button>
							<Button
								variant="default"
								size="sm"
								class="h-7"
								disabled={editBusy || !editDraft.trim()}
								onclick={() => void saveEdit()}
							>
								{#if editBusy}
									<LoaderCircle class="size-3.5 animate-spin" />
								{:else}
									保存
								{/if}
							</Button>
						</div>
						<p class="text-muted-foreground text-[10px]">Ctrl/⌘+Enter 保存 · Esc 取消 · 2 分钟内可编辑</p>
					</div>
				{:else}
					<div
						class={cn(
							'rounded-2xl px-3.5 py-2 text-sm leading-relaxed shadow-xs select-text',
							own
								? 'bg-primary text-primary-foreground rounded-br-md'
								: 'bg-muted text-foreground rounded-bl-md',
							voice && 'min-w-[12rem] py-2.5',
							failed && 'ring-destructive/50 opacity-80 ring-1',
							sending && 'opacity-80'
						)}
						title="右键打开菜单"
					>
						{#if hasReplyMeta}
							<div
								class={cn(
									'mb-1.5 flex w-full max-w-[18rem] flex-col rounded-lg border-l-2 px-2 py-1 text-left text-[11px]',
									own
										? 'border-primary-foreground/50 bg-primary-foreground/10 text-primary-foreground/90'
										: 'border-primary/50 bg-background/70 text-muted-foreground'
								)}
							>
								<span class="inline-flex items-center gap-1 font-semibold">
									<Reply class="size-3 opacity-80" />
									@{replyLabel}
								</span>
								{#if message.reply_to_preview}
									<span class="mt-0.5 line-clamp-2 opacity-90">{message.reply_to_preview}</span>
								{/if}
							</div>
						{/if}
						{#if voice}
							<div class="flex flex-col gap-1.5">
								<div class="flex items-center gap-2 text-xs font-medium opacity-90">
									<Mic class="size-3.5 shrink-0" />
									<span>{voiceLabel}</span>
								</div>
								{#if audioSrc}
									<audio
										controls
										preload="metadata"
										src={audioSrc}
										class={cn(
											'h-9 w-full max-w-[16rem]',
											own ? 'accent-primary-foreground' : 'accent-primary'
										)}
									>
										<track kind="captions" />
									</audio>
								{:else}
									<span class="text-xs opacity-80"
										>{message.content || 'Voice message unavailable'}</span
									>
								{/if}
							</div>
						{:else}
							{message.content}
						{/if}
					</div>
				{/if}
			</div>
			{#if own && (sending || failed)}
				<div
					class={cn(
						'flex items-center gap-1 px-1 text-[11px]',
						failed ? 'text-destructive' : 'text-muted-foreground'
					)}
				>
					{#if sending}
						<LoaderCircle class="size-3 animate-spin" />
						<span>发送中…网络波动会自动重试</span>
					{:else}
						<CircleAlert class="size-3" />
						<button
							type="button"
							class="underline-offset-2 hover:underline"
							onclick={() => onResend?.(message)}
						>
							发送失败，点击重试
						</button>
					{/if}
				</div>
			{/if}
		</div>
	</div>
{/if}

{#if menuOpen}
	<!-- Floating context menu -->
	<div
		data-msg-ctx-menu
		class="bg-popover text-popover-foreground fixed z-[200] min-w-[10.5rem] overflow-hidden rounded-xl border py-1 shadow-xl"
		style="left: {menuX}px; top: {menuY}px;"
		role="menu"
	>
		{#if showReply}
			<button
				type="button"
				role="menuitem"
				class="hover:bg-accent flex w-full items-center gap-2 px-3 py-2 text-left text-sm"
				onclick={doReply}
			>
				<Reply class="size-3.5 opacity-70" />
				回复
			</button>
		{/if}
		{#if showEdit}
			<button
				type="button"
				role="menuitem"
				class="hover:bg-accent flex w-full items-center gap-2 px-3 py-2 text-left text-sm"
				onclick={startEdit}
			>
				<Pencil class="size-3.5 opacity-70" />
				编辑
				<span class="text-muted-foreground ml-auto text-[10px]">2分钟内</span>
			</button>
		{/if}
		{#if showRecall}
			<button
				type="button"
				role="menuitem"
				class="hover:bg-accent text-destructive flex w-full items-center gap-2 px-3 py-2 text-left text-sm"
				onclick={doRecall}
			>
				<Undo2 class="size-3.5 opacity-70" />
				撤回
				<span class="text-muted-foreground ml-auto text-[10px]">2分钟内</span>
			</button>
		{/if}
		{#if failed && own && onResend}
			<button
				type="button"
				role="menuitem"
				class="hover:bg-accent flex w-full items-center gap-2 px-3 py-2 text-left text-sm"
				onclick={() => {
					closeMenu();
					onResend?.(message);
				}}
			>
				<RotateCcw class="size-3.5 opacity-70" />
				重新发送
			</button>
		{/if}
	</div>
{/if}
