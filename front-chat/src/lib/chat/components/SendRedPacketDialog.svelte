<script lang="ts">
	import type { ChatMode } from '../types';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import X from '@lucide/svelte/icons/x';
	import Coins from '@lucide/svelte/icons/coins';

	interface Props {
		open: boolean;
		chatMode: ChatMode;
		balance: number;
		onClose: () => void;
		onSend: (opts: {
			total_amount: number;
			total_count?: number;
			greeting?: string;
		}) => Promise<void>;
	}

	let { open, chatMode, balance, onClose, onSend }: Props = $props();

	let amount = $state('10');
	let count = $state('1');
	let greeting = $state('恭喜发财，大吉大利');
	let busy = $state(false);
	let errorMsg = $state('');

	$effect(() => {
		if (open) {
			errorMsg = '';
			busy = false;
			if (chatMode === 'private') count = '1';
		}
	});

	async function submit(e: Event) {
		e.preventDefault();
		const total_amount = Math.floor(Number(amount));
		const total_count = chatMode === 'group' ? Math.floor(Number(count)) : 1;
		if (!Number.isFinite(total_amount) || total_amount <= 0) {
			errorMsg = '请输入有效金额';
			return;
		}
		if (total_amount > balance) {
			errorMsg = `余额不足（当前 ${balance}）`;
			return;
		}
		if (chatMode === 'group') {
			if (!Number.isFinite(total_count) || total_count < 1) {
				errorMsg = '红包个数至少为 1';
				return;
			}
			if (total_amount < total_count) {
				errorMsg = '金额不能小于个数';
				return;
			}
		}
		busy = true;
		errorMsg = '';
		try {
			await onSend({
				total_amount,
				total_count: chatMode === 'group' ? total_count : undefined,
				greeting: greeting.trim() || '恭喜发财'
			});
			onClose();
		} catch (err) {
			errorMsg = (err as Error).message || '发送失败';
		} finally {
			busy = false;
		}
	}
</script>

{#if open}
	<div
		class="fixed inset-0 z-50 flex items-end justify-center bg-black/60 p-0 backdrop-blur-[2px] sm:items-center sm:p-4"
		role="dialog"
		aria-modal="true"
		tabindex="-1"
		onclick={(e) => {
			if (e.target === e.currentTarget) onClose();
		}}
		onkeydown={(e) => {
			if (e.key === 'Escape') onClose();
		}}
	>
		<div
			class="relative w-full max-w-md overflow-hidden rounded-t-3xl shadow-2xl sm:rounded-3xl"
		>
			<!-- red envelope header -->
			<div
				class="relative bg-gradient-to-b from-[#f24e3c] via-[#e03224] to-[#c41e12] px-5 pt-5 pb-8 text-white"
			>
				<button
					type="button"
					class="absolute top-3 right-3 flex size-8 items-center justify-center rounded-full bg-black/20 text-white/90 hover:bg-black/30"
					onclick={onClose}
					aria-label="关闭"
				>
					<X class="size-4" />
				</button>

				<div class="flex items-center gap-3">
					<div
						class="flex size-14 items-center justify-center rounded-full bg-gradient-to-br from-[#ffe9a8] via-[#f0c040] to-[#c99212] shadow-lg ring-2 ring-[#fff2b8]/40"
					>
						<span
							class="text-2xl font-bold text-[#8b1a12]"
							style="font-family: 'Songti SC', 'STSong', serif;">福</span
						>
					</div>
					<div>
						<p class="text-lg font-semibold tracking-wide">发红包</p>
						<p class="mt-0.5 text-xs text-amber-100/85">
							{chatMode === 'group' ? '拼手气红包 · 金额随机' : '普通红包 · 对方全领'}
						</p>
					</div>
				</div>

				<!-- wave bottom -->
				<svg
					class="absolute inset-x-0 bottom-0 h-5 w-full text-[#fff8f0]"
					viewBox="0 0 400 24"
					preserveAspectRatio="none"
					aria-hidden="true"
				>
					<path fill="currentColor" d="M0,24 L0,10 Q100,0 200,10 Q300,20 400,8 L400,24 Z" />
				</svg>
			</div>

			<form class="space-y-4 bg-[#fff8f0] px-5 pt-2 pb-5 dark:bg-[#1a1210]" onsubmit={submit}>
				<div
					class="rounded-2xl border border-red-200/60 bg-white px-4 py-3 shadow-sm dark:border-red-900/40 dark:bg-[#241816]"
				>
					<label
						class="text-muted-foreground mb-1.5 block text-[11px] font-medium tracking-wide"
						for="rp-amount"
					>
						总金额
					</label>
					<div class="flex items-center gap-2">
						<span class="text-xl font-semibold text-red-600">¥</span>
						<input
							id="rp-amount"
							type="number"
							min="1"
							step="1"
							bind:value={amount}
							class="min-w-0 flex-1 border-0 bg-transparent text-2xl font-semibold tracking-tight outline-none"
							placeholder="0"
						/>
						<span class="text-muted-foreground text-sm">币</span>
					</div>
					<p class="text-muted-foreground mt-2 flex items-center gap-1 text-[11px]">
						<Coins class="size-3 text-amber-500" />
						可用余额 {balance}
					</p>
				</div>

				{#if chatMode === 'group'}
					<div
						class="rounded-2xl border border-red-200/60 bg-white px-4 py-3 shadow-sm dark:border-red-900/40 dark:bg-[#241816]"
					>
						<label
							class="text-muted-foreground mb-1.5 block text-[11px] font-medium tracking-wide"
							for="rp-count"
						>
							红包个数
						</label>
						<div class="flex items-center gap-2">
							<input
								id="rp-count"
								type="number"
								min="1"
								step="1"
								bind:value={count}
								class="min-w-0 flex-1 border-0 bg-transparent text-xl font-semibold outline-none"
							/>
							<span class="text-muted-foreground text-sm">个</span>
						</div>
					</div>
				{/if}

				<div
					class="rounded-2xl border border-red-200/60 bg-white px-4 py-3 shadow-sm dark:border-red-900/40 dark:bg-[#241816]"
				>
					<label
						class="text-muted-foreground mb-1.5 block text-[11px] font-medium tracking-wide"
						for="rp-greet"
					>
						祝福语
					</label>
					<input
						id="rp-greet"
						maxlength={40}
						bind:value={greeting}
						class="w-full border-0 bg-transparent text-base outline-none"
						placeholder="恭喜发财，大吉大利"
					/>
				</div>

				{#if errorMsg}
					<p class="text-center text-xs text-red-600">{errorMsg}</p>
				{/if}

				<!-- preview mini envelope -->
				<div class="flex justify-center py-1">
					<div
						class="w-36 overflow-hidden rounded-xl shadow-md ring-1 ring-red-900/10"
					>
						<div
							class="bg-gradient-to-b from-[#f04a3a] to-[#c02218] px-3 py-2.5 text-center text-white"
						>
							<div
								class="mx-auto mb-1.5 flex size-8 items-center justify-center rounded-full bg-gradient-to-br from-[#ffe9a8] to-[#d4a017] text-xs font-bold text-[#8b1a12]"
								style="font-family: 'Songti SC', serif;"
							>
								福
							</div>
							<p class="truncate text-[11px]">{greeting || '恭喜发财'}</p>
						</div>
						<div class="bg-[#a81e14] py-1 text-center text-[9px] text-amber-100/70">
							预览
						</div>
					</div>
				</div>

				<Button
					type="submit"
					class="h-12 w-full rounded-full bg-gradient-to-r from-[#f04a3a] via-[#e03224] to-[#c41e12] text-base font-semibold text-white shadow-lg hover:brightness-110"
					disabled={busy}
				>
					{#if busy}
						<LoaderCircle class="size-5 animate-spin" />
						塞进红包…
					{:else}
						塞钱进红包
					{/if}
				</Button>
			</form>
		</div>
	</div>
{/if}
