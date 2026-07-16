<script lang="ts">
	import type { ChatMessage } from '../types';
	import { formatDuration, formatMessageLabel, isOwnMessage, isVoiceMessage } from '../utils';
	import { buildMediaUrl } from '$lib/api';
	import { cn } from '$lib/utils';
	import * as Avatar from '$lib/components/ui/avatar';
	import Mic from '@lucide/svelte/icons/mic';

	interface Props {
		message: ChatMessage;
		myUserId: string;
	}

	let { message, myUserId }: Props = $props();

	const own = $derived(isOwnMessage(message, myUserId));
	const voice = $derived(isVoiceMessage(message));
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
		voice && message.media_url ? buildMediaUrl(message.media_url) : ''
	);

	// Fallback label when media_url is missing / failed to resolve.
	const voiceLabel = $derived(
		`Voice · ${formatDuration(message.duration)}`
	);
</script>

<div class={cn('flex w-full gap-2', own ? 'flex-row-reverse' : 'flex-row')}>
	{#if !own}
		<Avatar.Root class="size-8 shrink-0">
			<Avatar.Fallback class="bg-muted text-muted-foreground text-xs font-medium">
				{initial}
			</Avatar.Fallback>
		</Avatar.Root>
	{/if}

	<div class={cn('flex max-w-[min(32rem,80%)] flex-col gap-1', own ? 'items-end' : 'items-start')}>
		<div class="text-muted-foreground flex items-center gap-2 px-1 text-[11px]">
			<span>{formatMessageLabel(message)}</span>
			{#if timeLabel}
				<span class="opacity-70">{timeLabel}</span>
			{/if}
		</div>
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
						<span class="text-xs opacity-80">{message.content || 'Voice message unavailable'}</span>
					{/if}
				</div>
			{:else}
				{message.content}
			{/if}
		</div>
	</div>
</div>
