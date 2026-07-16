<script lang="ts">
	import { onMount } from 'svelte';
	import { auth } from '$lib/auth.svelte';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';

	onMount(() => {
		if (auth.isAuthenticated) {
			window.location.href = '/chat';
		} else {
			window.location.href = '/login';
		}
	});
</script>

<div class="bg-background text-muted-foreground flex min-h-svh items-center justify-center gap-2">
	<LoaderCircle class="size-5 animate-spin" />
	<p class="text-sm">Loading…</p>
</div>
