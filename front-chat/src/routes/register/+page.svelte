<script lang="ts">
	import { onMount } from 'svelte';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';

	// Register UI lives on /login (tab switch). Keep this route so /register is not a 404.
	onMount(() => {
		window.location.replace('/login');
	});
</script>

<div class="bg-background text-muted-foreground flex min-h-svh items-center justify-center gap-2">
	<LoaderCircle class="size-5 animate-spin" />
	<p class="text-sm">Redirecting to login…</p>
</div>
