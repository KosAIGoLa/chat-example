<script lang="ts">
	import { auth } from '$lib/auth.svelte';
	import { api } from '$lib/api';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as Alert from '$lib/components/ui/alert';
	import MessageCircle from '@lucide/svelte/icons/message-circle';
	import CircleAlert from '@lucide/svelte/icons/circle-alert';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';

	let mode = $state<'login' | 'register'>('login');
	let username = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	async function handleSubmit(e: Event) {
		e.preventDefault();
		error = '';
		loading = true;
		try {
			if (mode === 'register') {
				await api.register(username, password);
			}
			const res = await api.login(username, password);
			auth.setAuth(res.token, res.user);
			window.location.href = '/chat';
		} catch (err) {
			error = (err as Error).message;
		} finally {
			loading = false;
		}
	}
</script>

<div
	class="bg-background flex min-h-svh flex-col items-center justify-center p-4"
>
	<div class="mb-8 flex flex-col items-center gap-3">
		<div
			class="bg-primary text-primary-foreground flex size-12 items-center justify-center rounded-xl shadow-sm"
		>
			<MessageCircle class="size-6" />
		</div>
		<div class="text-center">
			<h1 class="text-2xl font-semibold tracking-tight">WS Chat</h1>
			<p class="text-muted-foreground text-sm">Realtime messaging over WebSocket</p>
		</div>
	</div>

	<Card.Root class="w-full max-w-md shadow-lg">
		<Card.Header class="pb-4">
			<Card.Title class="text-lg">Welcome</Card.Title>
			<Card.Description>
				{mode === 'login' ? 'Sign in to continue' : 'Create an account to get started'}
			</Card.Description>
		</Card.Header>
		<Card.Content class="space-y-4">
			<Tabs.Root bind:value={mode} class="w-full">
				<Tabs.List class="grid w-full grid-cols-2">
					<Tabs.Trigger value="login">Login</Tabs.Trigger>
					<Tabs.Trigger value="register">Register</Tabs.Trigger>
				</Tabs.List>
			</Tabs.Root>

			<form onsubmit={handleSubmit} class="space-y-4">
				<div class="space-y-2">
					<Label for="username">Username</Label>
					<Input
						id="username"
						bind:value={username}
						required
						minlength={3}
						maxlength={50}
						autocomplete="username"
						placeholder="Enter username"
					/>
				</div>

				<div class="space-y-2">
					<Label for="password">Password</Label>
					<Input
						id="password"
						type="password"
						bind:value={password}
						required
						minlength={6}
						autocomplete={mode === 'login' ? 'current-password' : 'new-password'}
						placeholder="Enter password"
					/>
				</div>

				{#if error}
					<Alert.Root variant="destructive">
						<CircleAlert class="size-4" />
						<Alert.Title>Error</Alert.Title>
						<Alert.Description>{error}</Alert.Description>
					</Alert.Root>
				{/if}

				<Button type="submit" class="w-full" disabled={loading}>
					{#if loading}
						<LoaderCircle class="size-4 animate-spin" />
						Please wait…
					{:else if mode === 'login'}
						Login
					{:else}
						Register & Login
					{/if}
				</Button>
			</form>
		</Card.Content>
	</Card.Root>
</div>
