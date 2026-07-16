<script lang="ts">
	import { onMount } from 'svelte';
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';

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
	<link rel="icon" href={favicon} />
	<title>WS Chat</title>
</svelte:head>

{@render children()}
