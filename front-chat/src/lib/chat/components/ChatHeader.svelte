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

	interface Props {
		username: string;
		connectionStatus: ConnectionStatus;
		/** Current reconnect attempt (0 when idle / connected). */
		reconnectAttempt?: number;
		onLogout: () => void;
		/** Manual reconnect when stuck disconnected. */
		onReconnect?: () => void;
		/** Called after profile save so parent can refresh displayed name / token. */
		onProfileUpdated?: (username: string, token: string) => void;
	}

	let {
		username,
		connectionStatus,
		reconnectAttempt = 0,
		onLogout,
		onReconnect,
		onProfileUpdated
	}: Props = $props();

	let open = $state(false);
	let editUsername = $state('');
	let currentPassword = $state('');
	let newPassword = $state('');
	let saving = $state(false);
	let errorMsg = $state('');
	let successMsg = $state('');

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
			errorMsg = 'Username must be at least 3 characters';
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
			auth.setAuth(res.token, res.user);
			successMsg = 'Profile updated';
			onProfileUpdated?.(res.user.username, res.token);
			currentPassword = '';
			newPassword = '';
		} catch (err) {
			errorMsg = (err as Error).message || 'Update failed';
		} finally {
			saving = false;
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
						Reconnect
					</Button>
				{/if}
			</div>
		</div>

		<div class="flex items-center gap-2">
			<span class="text-muted-foreground hidden text-sm sm:inline">
				Signed in as <span class="text-foreground font-medium">{username}</span>
			</span>
			<Separator orientation="vertical" class="hidden h-5 sm:block" />
			<Button variant="outline" size="sm" onclick={openProfile} title="Edit profile">
				<UserCog class="size-4" />
				<span class="hidden sm:inline">Profile</span>
			</Button>
			<Button variant="outline" size="sm" onclick={onLogout}>
				<LogOut class="size-4" />
				<span class="hidden sm:inline">Logout</span>
			</Button>
		</div>
	</div>
</header>

<Sheet.Root bind:open>
	<Sheet.Content side="right" class="w-full sm:max-w-md">
		<Sheet.Header>
			<Sheet.Title>Edit profile</Sheet.Title>
			<Sheet.Description>Update your account name or password.</Sheet.Description>
		</Sheet.Header>

		<div class="flex flex-1 flex-col gap-4 px-4 pb-4">
			<div class="space-y-2">
				<Label for="profile-username">Username</Label>
				<Input id="profile-username" bind:value={editUsername} autocomplete="username" />
			</div>

			<Separator />

			<p class="text-muted-foreground text-xs font-medium tracking-wide uppercase">
				Change password (optional)
			</p>
			<div class="space-y-2">
				<Label for="profile-current-pw">Current password</Label>
				<Input
					id="profile-current-pw"
					type="password"
					bind:value={currentPassword}
					autocomplete="current-password"
					placeholder="Required only if setting a new password"
				/>
			</div>
			<div class="space-y-2">
				<Label for="profile-new-pw">New password</Label>
				<Input
					id="profile-new-pw"
					type="password"
					bind:value={newPassword}
					autocomplete="new-password"
					placeholder="Leave blank to keep current"
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
			<Button variant="outline" onclick={() => (open = false)} disabled={saving}>Cancel</Button>
			<Button onclick={saveProfile} disabled={saving}>
				{#if saving}
					<LoaderCircle class="size-4 animate-spin" />
					Saving…
				{:else}
					Save
				{/if}
			</Button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>
