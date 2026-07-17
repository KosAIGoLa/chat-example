<script lang="ts">
	import type { ChatMessage } from '../types';
	import {
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
	import RedPacketCard from './RedPacketCard.svelte';
	import UserAvatar from './UserAvatar.svelte';

	interface Props {
		message: ChatMessage;
		myUserId: string;
		/** Optional display name for message.from (username). */
		fromName?: string;
		/** Optional avatar image URL for message.from. */
		avatarSrc?: string;
		onRecall?: (msg: ChatMessage) => void;
		onResend?: (msg: ChatMessage) => void;
		onBalanceChange?: (balance: number) => void;
	}

	let {
		message,
		myUserId,
		fromName = '',
		avatarSrc = '',
		onRecall,
		onResend,
		onBalanceChange
	}: Props = $props();

	const own = $derived(isOwnMessage(message, myUserId));
	const voice = $derived(isVoiceMessage(message));
	const system = $derived(isSystemMessage(message));
	const redPacket = $derived(isRedPacketMessage(message));
	const recalled = $derived(!!message.recalled);
	const showRecall = $derived(
		canRecallMessage(message, myUserId) &&
			!!onRecall &&
			message.send_status !== 'sending' &&
			message.send_status !== 'failed' &&
			message.send_status !== 'pending'
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
	<div class={cn('group flex w-full items-end gap-2.5', own ? 'flex-row-reverse' : 'flex-row')}>
		<!-- Always show letter avatar; photo only overlays when URL loads -->
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
			</div>
			<div class="flex items-end gap-1">
				{#if showRecall && own && !redPacket}
					<Button
						variant="ghost"
						size="icon-xs"
						class="text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100"
						title="撤回 (2 分钟内)"
						onclick={() => onRecall?.(message)}
					>
						<Undo2 class="size-3.5" />
					</Button>
				{/if}
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
						<RedPacketCard {message} {myUserId} {own} {onBalanceChange} />
					</div>
				{:else}
					<div
						class={cn(
							'rounded-2xl px-3.5 py-2 text-sm leading-relaxed shadow-xs',
							own
								? 'bg-primary text-primary-foreground rounded-br-md'
								: 'bg-muted text-foreground rounded-bl-md',
							voice && 'min-w-[12rem] py-2.5',
							failed && 'ring-destructive/50 opacity-80 ring-1',
							sending && 'opacity-80'
						)}
					>
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
