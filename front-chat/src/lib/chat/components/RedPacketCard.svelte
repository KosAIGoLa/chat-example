<script lang="ts">
	import type { ChatMessage } from '../types';
	import { parseRedPacketContent } from '../utils';
	import { redPacketService, type RedPacket } from '$lib/api';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import X from '@lucide/svelte/icons/x';
	import { cn } from '$lib/utils';

	interface Props {
		message: ChatMessage;
		myUserId: string;
		own: boolean;
		onBalanceChange?: (balance: number) => void;
	}

	let { message, myUserId, own, onBalanceChange }: Props = $props();

	const packetId = $derived(message.red_packet_id || '');
	const parsed = $derived(parseRedPacketContent(message.content || ''));
	const isGroup = $derived(
		parsed.packet_type === 'group' || message.type === 'group' || !!message.group_id
	);

	let detail = $state<RedPacket | null>(null);
	let loading = $state(false);
	let claiming = $state(false);
	let errorMsg = $state('');
	let claimResult = $state<number | null>(null);
	let open = $state(false);
	/** short pulse when open button is pressed */
	let opening = $state(false);

	const myClaim = $derived(detail?.my_claim_amount || claimResult || 0);
	const finished = $derived(
		detail?.status === 'finished' ||
			detail?.status === 'refunded' ||
			detail?.status === 'expired' ||
			(detail != null && detail.remaining_count <= 0)
	);
	const claimedByMe = $derived(myClaim > 0);
	const openedLook = $derived(finished || claimedByMe);

	const statusLabel = $derived.by(() => {
		if (claimedByMe) return `已领取 ${myClaim} 币`;
		if (detail?.status === 'refunded' || detail?.status === 'expired') return '已过期';
		if (finished) return '已被领完';
		if (isGroup) return '拼手气红包';
		return '查看红包';
	});

	const senderLabel = $derived(own ? '你' : message.from || '好友');

	async function loadDetail() {
		if (!packetId) return;
		loading = true;
		errorMsg = '';
		try {
			detail = await redPacketService.get(packetId);
		} catch (e) {
			errorMsg = (e as Error).message || '加载失败';
		} finally {
			loading = false;
		}
	}

	async function onOpen() {
		open = true;
		opening = false;
		await loadDetail();
	}

	async function onClaim() {
		if (!packetId || claiming) return;
		claiming = true;
		opening = true;
		errorMsg = '';
		try {
			const res = await redPacketService.claim(packetId);
			claimResult = res.amount;
			onBalanceChange?.(res.balance);
			await loadDetail();
		} catch (e) {
			errorMsg = (e as Error).message || '领取失败';
			await loadDetail().catch(() => undefined);
		} finally {
			claiming = false;
			setTimeout(() => {
				opening = false;
			}, 400);
		}
	}

	const canClaim = $derived.by(() => {
		if (!detail) return false;
		if (detail.status !== 'open') return false;
		if ((detail.my_claim_amount ?? 0) > 0 || claimResult != null) return false;
		if (detail.type === 'private' && detail.to_user_id !== myUserId) return false;
		if (own && detail.type === 'private') return false;
		return detail.remaining_count > 0;
	});
</script>

<!-- Chat bubble: classic red envelope -->
<div class={cn('w-[15.5rem] max-w-[80vw]', own ? 'ml-auto' : 'mr-auto')}>
	<button
		type="button"
		class={cn(
			'group relative w-full overflow-hidden rounded-[1.1rem] text-left shadow-lg transition',
			'focus-visible:ring-2 focus-visible:ring-amber-300/80 focus-visible:outline-none',
			'hover:scale-[1.02] active:scale-[0.99]',
			openedLook ? 'opacity-90' : ''
		)}
		onclick={() => void onOpen()}
	>
		<!-- envelope body -->
		<div
			class={cn(
				'relative overflow-hidden px-3.5 pt-3.5 pb-3',
				openedLook
					? 'bg-gradient-to-b from-[#c45c4a] via-[#a8483a] to-[#8f3a30]'
					: 'bg-gradient-to-b from-[#f04a3a] via-[#e03828] to-[#c02218]'
			)}
		>
			<!-- decorative top flap fold -->
			<div
				class="pointer-events-none absolute inset-x-0 top-0 h-10 opacity-40"
				style="background: linear-gradient(180deg, rgba(0,0,0,.18) 0%, transparent 100%);"
			></div>
			<!-- gold corner ornaments -->
			<div
				class="pointer-events-none absolute top-2 left-2 size-5 rounded-tl-md border-t-2 border-l-2 border-amber-300/50"
			></div>
			<div
				class="pointer-events-none absolute top-2 right-2 size-5 rounded-tr-md border-t-2 border-r-2 border-amber-300/50"
			></div>

			<div class="relative flex items-center gap-3">
				<!-- gold seal -->
				<div
					class={cn(
						'relative flex size-12 shrink-0 items-center justify-center rounded-full shadow-md',
						'bg-gradient-to-br from-[#ffe9a8] via-[#f5c542] to-[#d4a017]',
						'ring-2 ring-[#fff3c4]/50'
					)}
				>
					<span
						class="font-serif text-lg font-bold text-[#8b1a12]"
						style="font-family: 'Songti SC', 'STSong', 'Noto Serif SC', serif;"
					>
						{openedLook ? '開' : '福'}
					</span>
					<!-- shine -->
					<div
						class="pointer-events-none absolute inset-0 rounded-full bg-gradient-to-tr from-white/50 via-transparent to-transparent"
					></div>
				</div>

				<div class="min-w-0 flex-1 text-white">
					<p
						class="truncate text-[15px] font-medium tracking-wide drop-shadow-sm"
						style="font-family: 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif;"
					>
						{parsed.greeting || '恭喜发财'}
					</p>
					<p class="mt-1 text-[11px] tracking-wider text-amber-100/85">
						{isGroup ? '拼手气红包' : '红包'}
						{#if openedLook}
							· {statusLabel}
						{/if}
					</p>
				</div>
			</div>
		</div>

		<!-- bottom strip (WeChat-style footer) -->
		<div
			class={cn(
				'flex items-center justify-between px-3.5 py-1.5 text-[10px]',
				openedLook
					? 'bg-[#7a3228] text-amber-100/70'
					: 'bg-[#a81e14] text-amber-100/80'
			)}
		>
			<span>微信风格红包</span>
			<span>{openedLook ? '已拆开' : '点击领取'}</span>
		</div>
	</button>
</div>

<!-- Full-screen open envelope modal -->
{#if open}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-[2px]"
		role="dialog"
		aria-modal="true"
		tabindex="-1"
		onclick={(e) => {
			if (e.target === e.currentTarget) open = false;
		}}
		onkeydown={(e) => {
			if (e.key === 'Escape') open = false;
		}}
	>
		<div class="relative w-full max-w-[20rem]">
			<!-- close -->
			<button
				type="button"
				class="absolute -top-2 -right-2 z-20 flex size-8 items-center justify-center rounded-full bg-black/40 text-white/90 ring-1 ring-white/20 hover:bg-black/55"
				onclick={() => (open = false)}
				aria-label="关闭"
			>
				<X class="size-4" />
			</button>

			<!-- envelope shell -->
			<div
				class={cn(
					'relative overflow-hidden rounded-[1.75rem] shadow-2xl',
					'bg-gradient-to-b from-[#f24e3c] via-[#e03224] to-[#b81810]'
				)}
			>
				<!-- top decorative wave / flap -->
				<div class="relative h-28 overflow-hidden">
					<svg
						class="absolute inset-x-0 bottom-0 h-16 w-full text-[#c41e12]"
						viewBox="0 0 400 80"
						preserveAspectRatio="none"
						aria-hidden="true"
					>
						<path
							fill="currentColor"
							d="M0,80 L0,28 Q100,0 200,28 Q300,56 400,28 L400,80 Z"
						/>
					</svg>
					<div class="relative z-10 flex flex-col items-center pt-7 text-center text-white">
						<p class="text-xs tracking-[0.2em] text-amber-100/90">
							{senderLabel}发出的红包
						</p>
						<p
							class="mt-2 max-w-[14rem] px-4 text-lg font-medium tracking-wide drop-shadow"
							style="font-family: 'PingFang SC', 'Microsoft YaHei', sans-serif;"
						>
							{parsed.greeting || '恭喜发财，大吉大利'}
						</p>
					</div>
				</div>

				<div class="relative px-6 pt-2 pb-8">
					{#if loading && !detail}
						<div class="flex flex-col items-center gap-3 py-10 text-white/90">
							<LoaderCircle class="size-8 animate-spin text-amber-200" />
							<span class="text-sm">打开红包中…</span>
						</div>
					{:else if claimedByMe}
						<!-- success: amount reveal -->
						<div class="flex flex-col items-center py-4 text-center">
							<p class="text-xs tracking-widest text-amber-100/80">恭喜你领到</p>
							<div class="mt-3 flex items-end gap-1 text-[#ffe08a]">
								<span
									class="text-5xl font-bold tracking-tight drop-shadow-md"
									style="font-family: 'DIN Alternate', 'Helvetica Neue', system-ui, sans-serif;"
								>
									{myClaim}
								</span>
								<span class="mb-1.5 text-base font-medium text-amber-200">币</span>
							</div>
							<div
								class="mt-5 h-px w-24 bg-gradient-to-r from-transparent via-amber-300/60 to-transparent"
							></div>
							<p class="mt-4 text-[11px] text-white/70">
								{isGroup ? '拼手气红包' : '私聊红包'}
								{#if detail}
									· 共 {detail.total_amount} 币
								{/if}
							</p>
						</div>
					{:else if finished}
						<div class="flex flex-col items-center py-10 text-center text-white/90">
							<div
								class="flex size-16 items-center justify-center rounded-full bg-black/15 ring-2 ring-amber-300/30"
							>
								<span
									class="font-serif text-2xl text-amber-200"
									style="font-family: 'Songti SC', 'STSong', serif;">空</span
								>
							</div>
							<p class="mt-4 text-base font-medium">手慢了，红包派完了</p>
							<p class="mt-1 text-xs text-white/60">
								{#if detail?.status === 'refunded' || detail?.status === 'expired'}
									红包已过期退回
								{:else}
									下次早点来抢哦
								{/if}
							</p>
						</div>
					{:else}
						<!-- unopened: big 開 button -->
						<div class="flex flex-col items-center py-2">
							<button
								type="button"
								class={cn(
									'relative flex size-[5.5rem] items-center justify-center rounded-full',
									'bg-gradient-to-br from-[#ffe9a8] via-[#f0c040] to-[#c99212]',
									'shadow-[0_8px_28px_rgba(0,0,0,.35),inset_0_2px_8px_rgba(255,255,255,.45)]',
									'ring-4 ring-[#fff2b8]/35 transition',
									'hover:scale-105 active:scale-95',
									'disabled:opacity-70',
									opening && 'animate-pulse'
								)}
								disabled={!canClaim || claiming || loading}
								onclick={() => void onClaim()}
							>
								{#if claiming}
									<LoaderCircle class="size-8 animate-spin text-[#8b1a12]" />
								{:else}
									<span
										class="text-4xl font-bold text-[#8b1a12] drop-shadow-sm"
										style="font-family: 'Songti SC', 'STSong', 'Noto Serif SC', serif;"
										>開</span
									>
								{/if}
								<div
									class="pointer-events-none absolute inset-0 rounded-full bg-gradient-to-tr from-white/40 via-transparent to-transparent"
								></div>
							</button>
							<p class="mt-5 text-xs tracking-widest text-amber-100/85">
								{canClaim
									? isGroup
										? '拼手气 · 点击开红包'
										: '点击开红包'
									: own && !isGroup
										? '等待对方领取'
										: '无法领取'}
							</p>
							{#if detail && isGroup}
								<p class="mt-1 text-[11px] text-white/55">
									剩余 {detail.remaining_count}/{detail.total_count} 个
								</p>
							{/if}
						</div>
					{/if}

					{#if errorMsg}
						<p class="mt-2 text-center text-xs text-amber-100">{errorMsg}</p>
					{/if}

					<!-- claim list -->
					{#if detail?.claims?.length}
						<div class="mt-4 rounded-2xl bg-black/15 px-3 py-3 backdrop-blur-sm">
							<p class="mb-2 text-center text-[10px] tracking-widest text-amber-100/70">
								领取详情 · {detail.claims.length}/{detail.total_count}
							</p>
							<div class="max-h-36 space-y-2 overflow-y-auto pr-0.5">
								{#each detail.claims as c (c.user_id + String(c.created_at))}
									<div class="flex items-center justify-between text-xs text-white/90">
										<div class="flex min-w-0 items-center gap-2">
											<div
												class="flex size-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-amber-200 to-amber-500 text-[10px] font-bold text-[#8b1a12]"
											>
												{(c.username || c.user_id || '?').slice(0, 1).toUpperCase()}
											</div>
											<span class="truncate">{c.username || c.user_id}</span>
											{#if c.user_id === myUserId}
												<span class="text-[10px] text-amber-200/80">我</span>
											{/if}
										</div>
										<span class="shrink-0 font-semibold text-amber-200">{c.amount} 币</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}
				</div>

				<!-- bottom gold line -->
				<div
					class="h-1.5 bg-gradient-to-r from-[#c99212] via-[#ffe08a] to-[#c99212] opacity-90"
				></div>
			</div>
		</div>
	</div>
{/if}
