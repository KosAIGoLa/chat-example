<script lang="ts">
	import { onMount } from 'svelte';
	import './layout.css';
	import NotifyHost from '$lib/ui/NotifyHost.svelte';

	let { children } = $props();

	// Prefer dark chat UI; clear leftover Service Workers (e.g. Open WebUI on :3000).
	onMount(async () => {
		document.documentElement.classList.add('dark');
		if (!('serviceWorker' in navigator)) return;
		const regs = await navigator.serviceWorker.getRegistrations();
		await Promise.all(regs.map((r) => r.unregister()));
	});
</script>

<svelte:head>
	<link rel="icon" href="/favicon.ico" sizes="any" />
	<link rel="icon" type="image/png" href="/favicon-32x32.png" sizes="32x32" />
	<link rel="apple-touch-icon" href="/apple-touch-icon.png" />
	<title>WS Chat</title>
</svelte:head>

{@render children()}
<NotifyHost />
