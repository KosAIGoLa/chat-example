<script lang="ts">
	import type { ChatMessage } from '../types';
	import MessageBubble from './MessageBubble.svelte';
	import * as ScrollArea from '$lib/components/ui/scroll-area';
	import MessagesSquare from '@lucide/svelte/icons/messages-square';

	interface Props {
		messages: ChatMessage[];
		myUserId: string;
		loading?: boolean;
		onRecall?: (msg: ChatMessage) => void;
	}

	let { messages, myUserId, loading = false, onRecall }: Props = $props();
	let bottomEl: HTMLDivElement | undefined = $state();

	$effect(() => {
		void messages.length;
		void loading;
		if (bottomEl) {
			requestAnimationFrame(() => {
				bottomEl?.scrollIntoView({ behavior: 'smooth', block: 'end' });
			});
		}
	});
</script>

<div class="bg-background relative flex min-h-0 flex-1 flex-col">
	{#if loading}
		<div
			class="bg-background/80 text-muted-foreground absolute inset-x-0 top-0 z-10 border-b px-4 py-2 text-center text-xs backdrop-blur"
		>
			Loading history…
		</div>
	{/if}

	<ScrollArea.Root class="min-h-0 flex-1">
		<div class="flex flex-col gap-4 px-4 py-6 md:px-6">
			{#if messages.length === 0 && !loading}
				<div
					class="text-muted-foreground flex min-h-[50vh] flex-col items-center justify-center gap-3"
				>
					<div class="bg-muted flex size-12 items-center justify-center rounded-full">
						<MessagesSquare class="size-6 opacity-60" />
					</div>
					<div class="text-center">
						<p class="text-foreground text-sm font-medium">No messages yet</p>
						<p class="text-xs">Select a user or group to start chatting</p>
					</div>
				</div>
			{:else}
				{#each messages as msg, i (`${msg.id ?? ''}-${msg.timestamp ?? 0}-${msg.from}-${msg.to}-${msg.media_url ?? ''}-${msg.content}-${i}`)}
					<MessageBubble message={msg} {myUserId} {onRecall} />
				{/each}
			{/if}
			<div bind:this={bottomEl} class="h-px w-full shrink-0"></div>
		</div>
	</ScrollArea.Root>
</div>
