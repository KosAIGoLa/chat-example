<script lang="ts">
	import type { ChatMessage } from '../types';
	import {
		canRecallMessage,
		formatDuration,
		formatMessageLabel,
		isOwnMessage,
		isSystemMessage,
		isVoiceMessage
	} from '../utils';
	import { buildMediaUrl } from '$lib/api';
	import { cn } from '$lib/utils';
	import * as Avatar from '$lib/components/ui/avatar';
	import { Button } from '$lib/components/ui/button';
	import Mic from '@lucide/svelte/icons/mic';
	import Undo2 from '@lucide/svelte/icons/undo-2';

	interface Props {
		message: ChatMessage;
		myUserId: string;
		onRecall?: (msg: ChatMessage) => void;
	}

	let { message, myUserId, onRecall }: Props = $props();

	const own = $derived(isOwnMessage(message, myUserId));
	const voice = $derived(isVoiceMessage(message));
	const system = $derived(isSystemMessage(message));
	const recalled = $derived(!!message.recalled);
	const showRecall = $derived(canRecallMessage(message, myUserId) && !!onRecall);
	const initial = $derived((message.from || '?').slice(0, 1).toUpperCase());
	const timeLabel = $derived(
		message.timestamp
			? new Date(message.timestamp * 1000).toLocaleTimeString([], {
					hour: '2-digit',
					minute: '2-digit'
				})
			: ''
	);
	const audioSrc = $derived(
		voice && message.media_url && !recalled ? buildMediaUrl(message.media_url) : ''
	);

	const voiceLabel = $derived(`Voice · ${formatDuration(message.duration)}`);
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
	<div class={cn('group flex w-full gap-2', own ? 'flex-row-reverse' : 'flex-row')}>
		{#if !own}
			<Avatar.Root class="size-8 shrink-0">
				<Avatar.Fallback class="bg-muted text-muted-foreground text-xs font-medium">
					{initial}
				</Avatar.Fallback>
			</Avatar.Root>
		{/if}

		<div
			class={cn(
				'flex max-w-[min(32rem,80%)] flex-col gap-1',
				own ? 'items-end' : 'items-start'
			)}
		>
			<div class="text-muted-foreground flex items-center gap-2 px-1 text-[11px]">
				<span>{formatMessageLabel(message)}</span>
				{#if timeLabel}
					<span class="opacity-70">{timeLabel}</span>
				{/if}
			</div>
			<div class="flex items-end gap-1">
				{#if showRecall && own}
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
				<div
					class={cn(
						'rounded-2xl px-3.5 py-2 text-sm leading-relaxed shadow-xs',
						own
							? 'bg-primary text-primary-foreground rounded-br-md'
							: 'bg-muted text-foreground rounded-bl-md',
						voice && 'min-w-[12rem] py-2.5'
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
				{#if showRecall && !own}
					<!-- never for others -->
				{/if}
			</div>
		</div>
	</div>
{/if}
