<script lang="ts">
	import type { GroupInfo, GroupMember } from '../types';
	import { groupAvatarUrl } from '$lib/api/group.service';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as ScrollArea from '$lib/components/ui/scroll-area';
	import UserAvatar from './UserAvatar.svelte';
	import { alertDialog, alertError, confirmDialog } from '$lib/ui/notify.svelte';
	import Camera from '@lucide/svelte/icons/camera';
	import Crown from '@lucide/svelte/icons/crown';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import Shield from '@lucide/svelte/icons/shield';
	import ShieldOff from '@lucide/svelte/icons/shield-off';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import LogOut from '@lucide/svelte/icons/log-out';
	import Users from '@lucide/svelte/icons/users';
	import Hash from '@lucide/svelte/icons/hash';

	interface Props {
		groupId: string;
		meta?: GroupInfo | null;
		members: GroupMember[];
		myUserId: string;
		/** owner | admin may rename + avatar */
		canManage: boolean;
		/** owner only: dissolve + role changes */
		isOwner: boolean;
		onRename: (name: string) => Promise<void>;
		onUploadAvatar: (file: File) => Promise<void>;
		onSetRole: (userId: string, role: 'admin' | 'member') => Promise<void>;
		onDissolve: () => Promise<void>;
		onLeave: () => Promise<void>;
		onRefreshMembers: () => void;
	}

	let {
		groupId,
		meta = null,
		members,
		myUserId,
		canManage,
		isOwner,
		onRename,
		onUploadAvatar,
		onSetRole,
		onDissolve,
		onLeave,
		onRefreshMembers
	}: Props = $props();

	let nameDraft = $state('');
	let nameBusy = $state(false);
	let nameError = $state('');
	let avatarBusy = $state(false);
	let avatarError = $state('');
	let roleBusyId = $state('');
	let dangerBusy = $state(false);
	let fileInput: HTMLInputElement | undefined = $state();

	// Sync name draft when opening / switching group.
	$effect(() => {
		const n = meta?.name || groupId;
		nameDraft = n;
		nameError = '';
		avatarError = '';
	});

	const displayName = $derived(meta?.name || groupId);
	const avatarSrc = $derived(
		meta?.avatar || meta?.avatar_rev
			? groupAvatarUrl(groupId, meta?.avatar_rev)
			: ''
	);
	const hue = $derived.by(() => {
		let h = 0;
		const s = groupId;
		for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
		return (h % 300) + 20;
	});

	function roleLabel(role: string): string {
		if (role === 'owner') return '群主';
		if (role === 'admin') return '管理者';
		return '一般成员';
	}

	function avatarUserSrc(uid: string): string {
		return uid ? `/api/avatar/${encodeURIComponent(uid)}` : '';
	}

	async function saveName() {
		const n = nameDraft.trim();
		if (!n) {
			nameError = '群名不能为空';
			return;
		}
		if (n === (meta?.name || '').trim()) {
			nameError = '';
			return;
		}
		nameBusy = true;
		nameError = '';
		try {
			await onRename(n);
		} catch (err) {
			nameError = (err as Error).message || '改名失败';
		} finally {
			nameBusy = false;
		}
	}

	async function onFilePick(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		input.value = '';
		if (!file) return;
		if (!file.type.startsWith('image/')) {
			avatarError = '请选择图片（jpg/png/webp/gif）';
			return;
		}
		if (file.size > 2 * 1024 * 1024) {
			avatarError = '图片不能超过 2MB';
			return;
		}
		avatarBusy = true;
		avatarError = '';
		try {
			await onUploadAvatar(file);
		} catch (err) {
			avatarError = (err as Error).message || '上传失败';
		} finally {
			avatarBusy = false;
		}
	}

	async function toggleRole(m: GroupMember) {
		if (!isOwner || m.user_id === myUserId || m.role === 'owner') return;
		const next: 'admin' | 'member' = m.role === 'admin' ? 'member' : 'admin';
		const ok = await confirmDialog({
			title: next === 'admin' ? '升级为管理者' : '降为一般成员',
			message:
				next === 'admin'
					? `将「${m.username || m.user_id}」升级为管理者？`
					: `将「${m.username || m.user_id}」降为一般成员？`,
			confirmText: next === 'admin' ? '升级' : '降级'
		});
		if (!ok) return;
		roleBusyId = m.user_id;
		try {
			await onSetRole(m.user_id, next);
		} catch (err) {
			await alertError((err as Error).message || '修改角色失败');
		} finally {
			roleBusyId = '';
		}
	}

	async function handleDissolve() {
		// Extra guard: only owner UI should call this.
		if (!isOwner) {
			await alertDialog({
				title: '无权操作',
				message: '仅群主可以解散群',
				kind: 'warning'
			});
			return;
		}
		const ok = await confirmDialog({
			title: '解散群聊',
			message: `解散群「${displayName}」？所有成员将被移除，此操作不可恢复。`,
			confirmText: '解散',
			danger: true
		});
		if (!ok) return;
		dangerBusy = true;
		try {
			await onDissolve();
		} catch (err) {
			await alertError((err as Error).message || '解散失败');
		} finally {
			dangerBusy = false;
		}
	}

	async function handleLeave() {
		const ok = await confirmDialog({
			title: '退出群聊',
			message: `退出群「${displayName}」？退出后将不再接收该群消息。`,
			confirmText: '退出',
			danger: true
		});
		if (!ok) return;
		dangerBusy = true;
		try {
			await onLeave();
		} catch (err) {
			await alertError((err as Error).message || '退出失败');
		} finally {
			dangerBusy = false;
		}
	}
</script>

<div class="flex h-full min-h-0 flex-col" aria-label="群配置">
	<ScrollArea.Root class="min-h-0 flex-1">
		<div class="space-y-6 px-4 py-4">
			<!-- 群图片 + 基本信息 -->
			<section class="flex flex-col items-center gap-3 pt-1">
				<div class="relative">
					<div
						class="relative flex size-20 items-center justify-center overflow-hidden rounded-2xl text-2xl font-semibold text-white shadow-md"
						style="background: hsl({hue} 55% 42%)"
					>
						{#if avatarSrc}
							<img
								src={avatarSrc}
								alt=""
								class="absolute inset-0 size-full object-cover"
								onerror={(e) => {
									(e.currentTarget as HTMLImageElement).style.display = 'none';
								}}
							/>
						{/if}
						<span class="relative z-0">{displayName.slice(0, 1).toUpperCase()}</span>
					</div>
					{#if canManage}
						<button
							type="button"
							class="bg-background absolute -right-1 -bottom-1 flex size-8 items-center justify-center rounded-full border shadow-sm hover:bg-muted disabled:opacity-60"
							title="更换群图片"
							disabled={avatarBusy}
							onclick={() => fileInput?.click()}
						>
							{#if avatarBusy}
								<LoaderCircle class="size-4 animate-spin" />
							{:else}
								<Camera class="size-4" />
							{/if}
						</button>
						<input
							bind:this={fileInput}
							type="file"
							accept="image/jpeg,image/png,image/webp,image/gif"
							class="hidden"
							onchange={(e) => void onFilePick(e)}
						/>
					{/if}
				</div>
				<div class="text-center">
					<p class="text-base font-semibold">{displayName}</p>
					<p class="text-muted-foreground mt-0.5 flex items-center justify-center gap-1 text-xs">
						<Hash class="size-3" />
						{groupId}
					</p>
					{#if typeof meta?.member_count === 'number'}
						<p class="text-muted-foreground mt-1 inline-flex items-center gap-1 text-xs">
							<Users class="size-3" />
							{meta.member_count} 位成员
						</p>
					{/if}
				</div>
				{#if avatarError}
					<p class="text-destructive text-center text-xs">{avatarError}</p>
				{/if}
				{#if canManage}
					<p class="text-muted-foreground text-center text-[11px]">
						点击相机图标更换群图片（jpg/png/webp/gif，≤2MB）
					</p>
				{/if}
			</section>

			<!-- 编辑群名 -->
			<section class="space-y-2">
				<Label for="group-settings-name" class="text-sm font-medium">群名称</Label>
				{#if canManage}
					<div class="flex gap-2">
						<Input
							id="group-settings-name"
							bind:value={nameDraft}
							maxlength={40}
							placeholder="2–40 个字符"
							disabled={nameBusy}
							onkeydown={(e) => {
								if (e.key === 'Enter') {
									e.preventDefault();
									void saveName();
								}
							}}
						/>
						<Button
							variant="default"
							size="sm"
							class="shrink-0"
							disabled={nameBusy || !nameDraft.trim() || nameDraft.trim() === (meta?.name || '').trim()}
							onclick={() => void saveName()}
						>
							{#if nameBusy}
								<LoaderCircle class="size-4 animate-spin" />
							{:else}
								保存
							{/if}
						</Button>
					</div>
					{#if nameError}
						<p class="text-destructive text-xs">{nameError}</p>
					{/if}
				{:else}
					<p class="bg-muted/50 rounded-lg border px-3 py-2 text-sm">{displayName}</p>
					<p class="text-muted-foreground text-[11px]">仅群主或管理者可修改群名与群图片</p>
				{/if}
			</section>

			<!-- 成员角色 -->
			<section class="space-y-2">
				<div class="flex items-center justify-between gap-2">
					<div>
						<p class="text-sm font-medium">成员与角色</p>
						<p class="text-muted-foreground text-[11px]">
							{#if isOwner}
								群主可将成员升级为管理者，或降为一般成员
							{:else}
								查看成员角色（群主 / 管理者 / 一般成员）
							{/if}
						</p>
					</div>
					<Button
						variant="ghost"
						size="sm"
						class="h-7 text-xs"
						onclick={onRefreshMembers}
					>
						刷新
					</Button>
				</div>
				<ul class="divide-border divide-y overflow-hidden rounded-xl border">
					{#each members as m (m.user_id)}
						{@const isMe = m.user_id === myUserId}
						<li class="flex items-center gap-2.5 px-3 py-2.5">
							<div class="relative shrink-0">
								<UserAvatar
									class="size-9"
									name={m.username || m.user_id}
									userId={m.user_id}
									src={avatarUserSrc(m.user_id)}
									alt={m.username}
								/>
								<span
									class="border-background absolute right-0 bottom-0 size-2.5 rounded-full border-2
										{m.online ? 'bg-emerald-500' : 'bg-muted-foreground/40'}"
									title={m.online ? '在线' : '离线'}
								></span>
							</div>
							<div class="min-w-0 flex-1">
								<div class="flex min-w-0 items-center gap-1.5">
									<span class="truncate text-sm font-medium">
										{m.username || m.user_id}
										{#if isMe}
											<span class="text-muted-foreground font-normal">(我)</span>
										{/if}
									</span>
									{#if m.role === 'owner'}
										<Badge
											variant="secondary"
											class="h-5 shrink-0 gap-0.5 border border-amber-500/30 bg-amber-500/10 px-1.5 text-[10px] font-medium text-amber-700 dark:text-amber-300"
										>
											<Crown class="size-3" />
											群主
										</Badge>
									{:else if m.role === 'admin'}
										<Badge
											variant="secondary"
											class="h-5 shrink-0 gap-0.5 border border-sky-500/30 bg-sky-500/10 px-1.5 text-[10px] font-medium text-sky-700 dark:text-sky-300"
										>
											<Shield class="size-3" />
											管理者
										</Badge>
									{:else}
										<Badge
											variant="outline"
											class="text-muted-foreground h-5 shrink-0 px-1.5 text-[10px] font-normal"
										>
											一般成员
										</Badge>
									{/if}
								</div>
								<p class="text-muted-foreground mt-0.5 text-[11px]">
									{m.online ? '在线' : '离线'} · {roleLabel(m.role)}
								</p>
							</div>
							{#if isOwner && !isMe && m.role !== 'owner'}
								{#if m.role === 'admin'}
									<Button
										variant="outline"
										size="sm"
										class="h-7 shrink-0 gap-1 px-2 text-xs"
										disabled={roleBusyId === m.user_id}
										title="降为一般成员"
										onclick={() => void toggleRole(m)}
									>
										{#if roleBusyId === m.user_id}
											<LoaderCircle class="size-3.5 animate-spin" />
										{:else}
											<ShieldOff class="size-3.5" />
										{/if}
										降级
									</Button>
								{:else}
									<Button
										variant="outline"
										size="sm"
										class="h-7 shrink-0 gap-1 px-2 text-xs"
										disabled={roleBusyId === m.user_id}
										title="升级为管理者"
										onclick={() => void toggleRole(m)}
									>
										{#if roleBusyId === m.user_id}
											<LoaderCircle class="size-3.5 animate-spin" />
										{:else}
											<Shield class="size-3.5" />
										{/if}
										升级
									</Button>
								{/if}
							{/if}
						</li>
					{:else}
						<li class="text-muted-foreground px-3 py-8 text-center text-sm">暂无成员</li>
					{/each}
				</ul>
			</section>

			<!-- 危险操作：解散仅群主；管理者/成员只能退出 -->
			<section class="space-y-2 border-t pt-4 pb-6">
				<p class="text-destructive text-sm font-medium">危险操作</p>
				{#if isOwner}
					<p class="text-muted-foreground text-[11px]">
						仅群主可解散。解散后群将被删除，所有成员失去访问权，不可恢复。
					</p>
					<Button
						variant="destructive"
						class="w-full gap-2"
						disabled={dangerBusy}
						onclick={() => void handleDissolve()}
					>
						{#if dangerBusy}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							<Trash2 class="size-4" />
						{/if}
						解散群
					</Button>
				{:else}
					<p class="text-muted-foreground text-[11px]">
						{#if canManage}
							管理者可改群名/群图，但无权解散群。退出后将不再接收该群消息。
						{:else}
							退出后将不再接收该群消息，可再次搜索加入。
						{/if}
					</p>
					<Button
						variant="outline"
						class="text-destructive border-destructive/40 hover:bg-destructive/10 w-full gap-2"
						disabled={dangerBusy}
						onclick={() => void handleLeave()}
					>
						{#if dangerBusy}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							<LogOut class="size-4" />
						{/if}
						退出群
					</Button>
				{/if}
			</section>
		</div>
	</ScrollArea.Root>
</div>
