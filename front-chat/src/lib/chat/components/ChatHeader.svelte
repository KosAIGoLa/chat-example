<script lang="ts">
	import type { ConnectionStatus } from '../types';
	import { authService } from '$lib/api';
	import { auth } from '$lib/auth.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';
	import * as Sheet from '$lib/components/ui/sheet';
	import MessageCircle from '@lucide/svelte/icons/message-circle';
	import LogOut from '@lucide/svelte/icons/log-out';
	import Wifi from '@lucide/svelte/icons/wifi';
	import WifiOff from '@lucide/svelte/icons/wifi-off';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import UserCog from '@lucide/svelte/icons/user-cog';
	import Coins from '@lucide/svelte/icons/coins';
	import Camera from '@lucide/svelte/icons/camera';
	import UserAvatar from './UserAvatar.svelte';

	interface Props {
		username: string;
		connectionStatus: ConnectionStatus;
		/** Current reconnect attempt (0 when idle / connected). */
		reconnectAttempt?: number;
		/** Virtual wallet balance. */
		balance?: number;
		onLogout: () => void;
		/** Manual reconnect when stuck disconnected. */
		onReconnect?: () => void;
		/** Called after profile save so parent can refresh displayed name / token. */
		onProfileUpdated?: (username: string, token: string) => void;
		/** Called after avatar upload (parent may force avatar URL refresh). */
		onAvatarUpdated?: (avatarUrl: string, rev: number) => void;
	}

	let {
		username,
		connectionStatus,
		reconnectAttempt = 0,
		balance = 0,
		onLogout,
		onReconnect,
		onProfileUpdated,
		onAvatarUpdated
	}: Props = $props();

	let open = $state(false);
	let editUsername = $state('');
	let currentPassword = $state('');
	let newPassword = $state('');
	let saving = $state(false);
	let uploadingAvatar = $state(false);
	let errorMsg = $state('');
	let successMsg = $state('');
	let fileInput: HTMLInputElement | undefined = $state();

	const statusVariant = $derived(
		connectionStatus === 'connected'
			? 'default'
			: connectionStatus === 'connecting' || connectionStatus === 'reconnecting'
				? 'secondary'
				: 'destructive'
	);

	const statusLabel = $derived(
		connectionStatus === 'reconnecting'
			? reconnectAttempt > 0
				? `reconnecting · ${reconnectAttempt}`
				: 'reconnecting'
			: connectionStatus
	);

	const myAvatarSrc = $derived.by(() => {
		const u = auth.user;
		if (!u?.id) return '';
		if (!u.avatar && !u.avatar_rev) return '';
		const rev = u.avatar_rev || 0;
		return `/api/avatar/${u.id}${rev ? `?v=${rev}` : ''}`;
	});

	function openProfile() {
		editUsername = username;
		currentPassword = '';
		newPassword = '';
		errorMsg = '';
		successMsg = '';
		open = true;
	}

	async function saveProfile() {
		const name = editUsername.trim();
		if (name.length < 3) {
			errorMsg = '用户名至少 3 个字符';
			return;
		}
		saving = true;
		errorMsg = '';
		successMsg = '';
		try {
			const body: {
				username: string;
				password?: string;
				current_password?: string;
			} = { username: name };
			if (newPassword) {
				body.password = newPassword;
				body.current_password = currentPassword;
			}
			const res = await authService.updateProfile(body);
			// Keep avatar fields from previous user if update profile doesn't return them.
			const merged = {
				...auth.user,
				...res.user,
				avatar: res.user.avatar ?? auth.user?.avatar,
				avatar_rev: res.user.avatar_rev ?? auth.user?.avatar_rev
			};
			auth.setAuth(res.token, merged);
			successMsg = '资料已更新';
			onProfileUpdated?.(res.user.username, res.token);
			currentPassword = '';
			newPassword = '';
		} catch (err) {
			errorMsg = (err as Error).message || '更新失败';
		} finally {
			saving = false;
		}
	}

	async function onPickAvatar(e: Event) {
		const input = e.target as HTMLInputElement;
		const file = input.files?.[0];
		input.value = '';
		if (!file) return;
		if (!file.type.startsWith('image/')) {
			errorMsg = '请选择图片文件（jpg/png/webp/gif）';
			return;
		}
		if (file.size > 2 * 1024 * 1024) {
			errorMsg = '图片不能超过 2MB';
			return;
		}
		uploadingAvatar = true;
		errorMsg = '';
		successMsg = '';
		try {
			const res = await authService.uploadAvatar(file);
			if (auth.user) {
				auth.updateUser({
					...auth.user,
					avatar: res.avatar,
					avatar_rev: res.avatar_rev
				});
			}
			successMsg = '头像已更新';
			onAvatarUpdated?.(res.url || res.avatar, res.avatar_rev);
		} catch (err) {
			errorMsg = (err as Error).message || '头像上传失败';
		} finally {
			uploadingAvatar = false;
		}
	}
</script>

<header class="bg-background/95 supports-backdrop-filter:bg-background/80 border-b backdrop-blur">
	<div class="flex h-14 items-center justify-between gap-3 px-4 md:px-6">
		<div class="flex items-center gap-3">
			<div
				class="bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-lg"
			>
				<MessageCircle class="size-4" />
			</div>
			<div class="flex items-center gap-2">
				<h1 class="text-sm font-semibold tracking-tight md:text-base">WS Chat</h1>
				<Badge variant={statusVariant} class="gap-1 font-normal capitalize">
					{#if connectionStatus === 'connected'}
						<Wifi class="size-3" />
					{:else if connectionStatus === 'connecting' || connectionStatus === 'reconnecting'}
						<LoaderCircle class="size-3 animate-spin" />
					{:else}
						<WifiOff class="size-3" />
					{/if}
					{statusLabel}
				</Badge>
				{#if connectionStatus === 'disconnected' && onReconnect}
					<Button variant="ghost" size="sm" class="h-7 px-2 text-xs" onclick={onReconnect}>
						重连
					</Button>
				{/if}
			</div>
		</div>

		<div class="flex items-center gap-2">
			<Badge
				variant="secondary"
				class="gap-1 border border-amber-500/30 bg-amber-500/10 font-medium text-amber-700 dark:text-amber-300"
				title="虚拟币余额"
			>
				<Coins class="size-3.5" />
				{balance}
			</Badge>
			<button
				type="button"
				class="hidden items-center gap-2 sm:flex"
				onclick={openProfile}
				title="个人资料"
			>
				<UserAvatar
					class="size-8 ring-1 ring-border"
					name={username || auth.user?.username || '?'}
					userId={String(auth.user?.id ?? '')}
					src={myAvatarSrc}
					primary
					alt={username}
				/>
				<span class="text-foreground text-sm font-medium">{username}</span>
			</button>
			<Separator orientation="vertical" class="hidden h-5 sm:block" />
			<Button variant="outline" size="sm" onclick={openProfile} title="个人资料">
				<UserCog class="size-4" />
				<span class="hidden sm:inline">资料</span>
			</Button>
			<Button variant="outline" size="sm" onclick={onLogout}>
				<LogOut class="size-4" />
				<span class="hidden sm:inline">退出</span>
			</Button>
		</div>
	</div>
</header>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>个人资料</Sheet.Title>
			<Sheet.Description>修改头像、用户名或密码。</Sheet.Description>
		</Sheet.Header>

		<div class="flex flex-1 flex-col gap-4 px-4 pb-4">
			<!-- Avatar upload -->
			<div class="flex flex-col items-center gap-3 py-2">
				<div class="relative">
					<UserAvatar
						class="size-24 shadow-md ring-2 ring-border"
						name={username || auth.user?.username || '?'}
						userId={String(auth.user?.id ?? '')}
						src={myAvatarSrc}
						primary
						textClass="text-2xl"
						alt={username}
					/>
					<button
						type="button"
						class="bg-background absolute right-0 bottom-0 flex size-8 items-center justify-center rounded-full border shadow-sm hover:bg-muted"
						disabled={uploadingAvatar}
						onclick={() => fileInput?.click()}
						title="更换头像"
						aria-label="更换头像"
					>
						{#if uploadingAvatar}
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
						onchange={onPickAvatar}
					/>
				</div>
				<p class="text-muted-foreground text-center text-xs">
					点击相机更换头像 · jpg / png / webp · 最大 2MB
				</p>
			</div>

			<Separator />

			<div class="space-y-2">
				<Label for="profile-username">用户名</Label>
				<Input id="profile-username" bind:value={editUsername} autocomplete="username" />
			</div>

			<Separator />

			<p class="text-muted-foreground text-xs font-medium tracking-wide uppercase">
				修改密码（可选）
			</p>
			<div class="space-y-2">
				<Label for="profile-current-pw">当前密码</Label>
				<Input
					id="profile-current-pw"
					type="password"
					bind:value={currentPassword}
					autocomplete="current-password"
					placeholder="仅在修改密码时需要"
				/>
			</div>
			<div class="space-y-2">
				<Label for="profile-new-pw">新密码</Label>
				<Input
					id="profile-new-pw"
					type="password"
					bind:value={newPassword}
					autocomplete="new-password"
					placeholder="留空则不修改"
				/>
			</div>

			{#if errorMsg}
				<p class="text-destructive text-sm">{errorMsg}</p>
			{/if}
			{#if successMsg}
				<p class="text-sm text-emerald-600">{successMsg}</p>
			{/if}
		</div>

		<Sheet.Footer class="gap-2 sm:flex-row">
			<Button variant="outline" onclick={() => (open = false)} disabled={saving}>取消</Button>
			<Button onclick={saveProfile} disabled={saving}>
				{#if saving}
					<LoaderCircle class="size-4 animate-spin" />
					保存中…
				{:else}
					保存
				{/if}
			</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
