<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import {
		COFFEE_ADDRESS,
		COFFEE_ASSET,
		COFFEE_LABEL,
		COFFEE_NETWORK
	} from '$lib/coffee';
	import { alertDialog, toastInfo } from '$lib/ui/notify.svelte';
	import Coffee from '@lucide/svelte/icons/coffee';

	interface Props {
		/** compact icon for header; full for login footer */
		variant?: 'icon' | 'full' | 'link';
		class?: string;
	}

	let { variant = 'icon', class: className = '' }: Props = $props();

	async function copyAddress() {
		try {
			await navigator.clipboard.writeText(COFFEE_ADDRESS);
			toastInfo('地址已复制到剪贴板', COFFEE_LABEL);
		} catch {
			// fall through to dialog
		}
		await alertDialog({
			title: COFFEE_LABEL,
			kind: 'info',
			okText: '知道了',
			message: `感谢支持 ☕\n\n网络：${COFFEE_NETWORK}\n资产：${COFFEE_ASSET}\n地址：\n${COFFEE_ADDRESS}\n\n（地址已尝试复制）`
		});
	}
</script>

{#if variant === 'full'}
	<Button
		type="button"
		variant="outline"
		class="h-10 gap-2 border-amber-500/40 bg-amber-500/10 text-amber-800 hover:bg-amber-500/15 dark:text-amber-200 {className}"
		onclick={() => void copyAddress()}
		title={COFFEE_LABEL}
	>
		<Coffee class="size-4" />
		{COFFEE_LABEL}
	</Button>
{:else if variant === 'link'}
	<button
		type="button"
		class="text-muted-foreground hover:text-amber-700 dark:hover:text-amber-300 inline-flex items-center gap-1.5 text-xs underline-offset-2 hover:underline {className}"
		onclick={() => void copyAddress()}
	>
		<Coffee class="size-3.5" />
		{COFFEE_LABEL}
	</button>
{:else}
	<Button
		type="button"
		variant="ghost"
		size="icon"
		class="text-amber-700 hover:bg-amber-500/15 hover:text-amber-800 dark:text-amber-300 {className}"
		onclick={() => void copyAddress()}
		title="{COFFEE_LABEL} · {COFFEE_NETWORK}"
		aria-label={COFFEE_LABEL}
	>
		<Coffee class="size-4" />
	</Button>
{/if}
