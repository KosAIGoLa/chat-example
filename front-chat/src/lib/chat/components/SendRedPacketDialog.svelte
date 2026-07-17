<script lang="ts">
	import type { ChatMode } from '../types';
	import { Button } from '$lib/components/ui/button';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import X from '@lucide/svelte/icons/x';
	import Coins from '@lucide/svelte/icons/coins';
	import Check from '@lucide/svelte/icons/check';
	import { cn } from '$lib/utils';

	interface Props {
		open: boolean;
		chatMode: ChatMode;
		balance: number;
		/** Group members for designated picker (group chat only). */
		members?: { user_id: string; username: string }[];
		myUserId?: string;
		onClose: () => void;
		onSend: (opts: {
			total_amount: number;
			total_count?: number;
			greeting?: string;
			type?: 'group' | 'designated';
			target_user_ids?: string[];
		}) => Promise<void>;
	}

	let { open, chatMode, balance, members = [], myUserId = '', onClose, onSend }: Props = $props();

	/** group: 拼手气; designated: 指定人 */
	let groupKind = $state<'group' | 'designated'>('group');
	let amount = $state('10');
	let count = $state('1');
	let greeting = $state('恭喜发财，大吉大利');
	let busy = $state(false);
	let errorMsg = $state('');
	/** selected user ids for designated (array avoids Set reactivity lint) */
	let selected = $state<string[]>([]);

	const selectableMembers = $derived(
		(members ?? []).filter((m) => m.user_id && m.user_id !== myUserId)
	);

	$effect(() => {
		if (open) {
			errorMsg = '';
			busy = false;
			groupKind = 'group';
			selected = [];
			if (chatMode === 'private') count = '1';
		}
	});

	function toggleMember(uid: string) {
		if (selected.includes(uid)) {
			selected = selected.filter((id) => id !== uid);
		} else {
			selected = [...selected, uid];
		}
		count = String(selected.length || 1);
	}

	function selectAll() {
		selected = selectableMembers.map((m) => m.user_id);
		count = String(selected.length || 1);
	}

	function clearSelection() {
		selected = [];
		count = '1';
	}

	async function submit(e: Event) {
		e.preventDefault();
		const total_amount = Math.floor(Number(amount));
		if (!Number.isFinite(total_amount) || total_amount <= 0) {
			errorMsg = '请输入有效金额';
			return;
		}
		if (total_amount > balance) {
			errorMsg = `余额不足（当前 ${balance}）`;
			return;
		}

		if (chatMode === 'private') {
			busy = true;
			errorMsg = '';
			try {
				await onSend({
					total_amount,
					greeting: greeting.trim() || '恭喜发财'
				});
				onClose();
			} catch (err) {
				errorMsg = (err as Error).message || '发送失败';
			} finally {
				busy = false;
			}
			return;
		}

		// group chat
		if (groupKind === 'designated') {
			const target_user_ids = [...selected];
			if (target_user_ids.length < 1) {
				errorMsg = '请至少选择一位领取人';
				return;
			}
			if (total_amount < target_user_ids.length) {
				errorMsg = '金额不能小于指定人数';
				return;
			}
			busy = true;
			errorMsg = '';
			try {
				await onSend({
					type: 'designated',
					total_amount,
					target_user_ids,
					greeting: greeting.trim() || '恭喜发财'
				});
				onClose();
			} catch (err) {
				errorMsg = (err as Error).message || '发送失败';
			} finally {
				busy = false;
			}
			return;
		}

		const total_count = Math.floor(Number(count));
		if (!Number.isFinite(total_count) || total_count < 1) {
			errorMsg = '红包个数至少为 1';
			return;
		}
		if (total_amount < total_count) {
			errorMsg = '金额不能小于个数';
			return;
		}
		busy = true;
		errorMsg = '';
		try {
			await onSend({
				type: 'group',
				total_amount,
				total_count,
				greeting: greeting.trim() || '恭喜发财'
			});
			onClose();
		} catch (err) {
			errorMsg = (err as Error).message || '发送失败';
		} finally {
			busy = false;
		}
	}

	const subtitle = $derived.by(() => {
		if (chatMode === 'private') return '普通红包 · 对方全领';
		if (groupKind === 'designated') return '指定红包 · 均分给选定成员';
		return '拼手气红包 · 金额随机';
	});
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
			class="relative max-h-[92vh] w-full max-w-md overflow-hidden overflow-y-auto rounded-t-3xl shadow-2xl sm:rounded-3xl"
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
						<p class="mt-0.5 text-xs text-amber-100/85">{subtitle}</p>
					</div>
				</div>

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
				{#if chatMode === 'group'}
					<div
						class="flex rounded-2xl border border-red-200/60 bg-white p-1 shadow-sm dark:border-red-900/40 dark:bg-[#241816]"
					>
						<button
							type="button"
							class={cn(
								'flex-1 rounded-xl py-2 text-sm font-medium transition',
								groupKind === 'group'
									? 'bg-gradient-to-r from-[#f04a3a] to-[#c41e12] text-white shadow'
									: 'text-muted-foreground hover:bg-red-50 dark:hover:bg-red-950/30'
							)}
							onclick={() => {
								groupKind = 'group';
								errorMsg = '';
							}}
						>
							拼手气
						</button>
						<button
							type="button"
							class={cn(
								'flex-1 rounded-xl py-2 text-sm font-medium transition',
								groupKind === 'designated'
									? 'bg-gradient-to-r from-[#f04a3a] to-[#c41e12] text-white shadow'
									: 'text-muted-foreground hover:bg-red-50 dark:hover:bg-red-950/30'
							)}
							onclick={() => {
								groupKind = 'designated';
								errorMsg = '';
								if (selected.length) count = String(selected.length);
							}}
						>
							指定红包
						</button>
					</div>
				{/if}

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

				{#if chatMode === 'group' && groupKind === 'group'}
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

				{#if chatMode === 'group' && groupKind === 'designated'}
					<div
						class="rounded-2xl border border-red-200/60 bg-white px-4 py-3 shadow-sm dark:border-red-900/40 dark:bg-[#241816]"
					>
						<div class="mb-2 flex items-center justify-between">
							<p class="text-muted-foreground text-[11px] font-medium tracking-wide">
								指定领取人 · 已选 {selected.length} 人
							</p>
							<div class="flex gap-2">
								<button
									type="button"
									class="text-[11px] font-medium text-red-600 hover:underline"
									onclick={selectAll}
								>
									全选
								</button>
								<button
									type="button"
									class="text-muted-foreground text-[11px] hover:underline"
									onclick={clearSelection}
								>
									清空
								</button>
							</div>
						</div>
						{#if selectableMembers.length === 0}
							<p class="text-muted-foreground py-4 text-center text-xs">暂无其他群成员</p>
						{:else}
							<div class="max-h-44 space-y-1 overflow-y-auto pr-0.5">
								{#each selectableMembers as m (m.user_id)}
									{@const checked = selected.includes(m.user_id)}
									<button
										type="button"
										class={cn(
											'flex w-full items-center gap-3 rounded-xl px-2 py-2 text-left transition',
											checked
												? 'bg-red-50 ring-1 ring-red-200 dark:bg-red-950/40 dark:ring-red-900/50'
												: 'hover:bg-muted/60'
										)}
										onclick={() => toggleMember(m.user_id)}
									>
										<div
											class={cn(
												'flex size-5 shrink-0 items-center justify-center rounded-md border transition',
												checked
													? 'border-red-500 bg-red-500 text-white'
													: 'border-muted-foreground/40'
											)}
										>
											{#if checked}
												<Check class="size-3.5" strokeWidth={3} />
											{/if}
										</div>
										<div
											class="flex size-8 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-amber-200 to-amber-500 text-xs font-bold text-[#8b1a12]"
										>
											{(m.username || m.user_id || '?').slice(0, 1).toUpperCase()}
										</div>
										<span class="min-w-0 truncate text-sm font-medium">
											{m.username || m.user_id}
										</span>
									</button>
								{/each}
							</div>
						{/if}
						{#if selected.length > 0}
							<p class="text-muted-foreground mt-2 text-[11px]">
								共 {selected.length} 份 · 每人约 {Math.floor(Math.max(1, Number(amount) || 0) / selected.length)} 币（均分）
							</p>
						{/if}
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

				<div class="flex justify-center py-1">
					<div class="w-36 overflow-hidden rounded-xl shadow-md ring-1 ring-red-900/10">
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
							{chatMode === 'group' && groupKind === 'designated'
								? `指定 ${selected.length || 0} 人`
								: '预览'}
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
