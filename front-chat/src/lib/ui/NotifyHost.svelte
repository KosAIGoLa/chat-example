<script lang="ts">
	import { Button } from '$lib/components/ui/button';
	import {
		dismissToast,
		getConfirmOptions,
		getToasts,
		isConfirmOpen,
		resolveConfirm,
		type ToastKind
	} from './notify.svelte';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import CircleCheck from '@lucide/svelte/icons/circle-check';
	import Info from '@lucide/svelte/icons/info';
	import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
	import X from '@lucide/svelte/icons/x';
	import { cn } from '$lib/utils';

	const toasts = $derived(getToasts());
	const confirmOpen = $derived(isConfirmOpen());
	const confirmOpts = $derived(getConfirmOptions());

	function kindClass(kind: ToastKind): string {
		switch (kind) {
			case 'success':
				return 'border-emerald-500/40 bg-emerald-950/90 text-emerald-50';
			case 'error':
				return 'border-red-500/40 bg-red-950/90 text-red-50';
			case 'warning':
				return 'border-amber-500/40 bg-amber-950/90 text-amber-50';
			default:
				return 'border-border bg-card text-card-foreground';
		}
	}
</script>

<!-- Toasts -->
<div
	class="pointer-events-none fixed inset-x-0 bottom-0 z-[100] flex flex-col items-center gap-2 p-4 sm:items-end"
	aria-live="polite"
>
	{#each toasts as t (t.id)}
		<div
			class={cn(
				'pointer-events-auto flex w-full max-w-sm items-start gap-3 rounded-xl border px-4 py-3 shadow-lg backdrop-blur',
				'animate-in fade-in slide-in-from-bottom-2 duration-200',
				kindClass(t.kind)
			)}
			role="status"
		>
			<div class="mt-0.5 shrink-0">
				{#if t.kind === 'success'}
					<CircleCheck class="size-4" />
				{:else if t.kind === 'error'}
					<CircleAlert class="size-4" />
				{:else if t.kind === 'warning'}
					<TriangleAlert class="size-4" />
				{:else}
					<Info class="size-4" />
				{/if}
			</div>
			<div class="min-w-0 flex-1">
				{#if t.title}
					<p class="text-sm font-semibold">{t.title}</p>
				{/if}
				<p class="text-sm leading-snug opacity-95">{t.message}</p>
			</div>
			<button
				type="button"
				class="shrink-0 rounded-md p-0.5 opacity-70 hover:opacity-100"
				onclick={() => dismissToast(t.id)}
				aria-label="关闭"
			>
				<X class="size-3.5" />
			</button>
		</div>
	{/each}
</div>

<!-- Confirm modal -->
{#if confirmOpen}
	<div
		class="bg-background/70 fixed inset-0 z-[110] flex items-center justify-center p-4 backdrop-blur-sm"
		role="dialog"
		aria-modal="true"
		aria-labelledby="confirm-title"
	>
		<div class="bg-card w-full max-w-sm rounded-2xl border p-5 shadow-2xl">
			<h2 id="confirm-title" class="text-base font-semibold">
				{confirmOpts.title ?? '请确认'}
			</h2>
			<p class="text-muted-foreground mt-2 text-sm leading-relaxed">
				{confirmOpts.message}
			</p>
			<div class="mt-5 flex justify-end gap-2">
				<Button variant="outline" onclick={() => resolveConfirm(false)}>
					{confirmOpts.cancelText ?? '取消'}
				</Button>
				<Button
					variant={confirmOpts.danger ? 'destructive' : 'default'}
					onclick={() => resolveConfirm(true)}
				>
					{confirmOpts.confirmText ?? '确定'}
				</Button>
			</div>
		</div>
	</div>
{/if}
