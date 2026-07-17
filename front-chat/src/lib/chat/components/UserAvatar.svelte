<script lang="ts">
	import { avatarInitials } from '../utils';
	import { cn } from '$lib/utils';

	interface Props {
		/** Display name used for initials when no photo. */
		name?: string;
		/** Stable id for color hash (user_id preferred). */
		userId?: string;
		/** Optional photo URL; empty / 404 → letter avatar only. */
		src?: string;
		/** CSS size classes, e.g. size-10 */
		class?: string;
		/** Use primary theme color (own messages / self). */
		primary?: boolean;
		/** Extra classes on the initials text. */
		textClass?: string;
		alt?: string;
	}

	let {
		name = '',
		userId = '',
		src = '',
		class: className = 'size-10',
		primary = false,
		textClass = '',
		alt = ''
	}: Props = $props();

	/** Only set after Image() proves the URL is a real picture. */
	let photoSrc = $state('');

	const label = $derived((name || userId || '?').trim() || '?');
	const initials = $derived(avatarInitials(label, 2));
	const hue = $derived.by(() => {
		const s = String(userId || name || '0');
		let h = 0;
		for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
		// Prefer non-harsh hues (skip pure red band a bit).
		return (h % 300) + 20;
	});

	// Probe photo URL off-DOM so 404 / tiny junk never covers the letter avatar.
	// Reject images smaller than 32px (e.g. accidental 8×8 solid-color uploads).
	$effect(() => {
		const url = (src || '').trim();
		photoSrc = '';
		if (!url) return;

		let cancelled = false;
		const img = new Image();
		img.onload = () => {
			if (cancelled) return;
			const w = img.naturalWidth;
			const h = img.naturalHeight;
			if (w >= 32 && h >= 32) {
				photoSrc = url;
			} else {
				// Treat tiny / solid placeholders as “no avatar”.
				photoSrc = '';
			}
		};
		img.onerror = () => {
			if (!cancelled) photoSrc = '';
		};
		img.src = url;

		return () => {
			cancelled = true;
			img.onload = null;
			img.onerror = null;
		};
	});
</script>

<div
	class={cn(
		'relative flex shrink-0 select-none items-center justify-center overflow-hidden rounded-full shadow-sm ring-2 ring-background',
		primary && !photoSrc && 'bg-primary',
		className
	)}
	title={label}
	role="img"
	aria-label={alt || label}
	style={!photoSrc && !primary
		? `background: linear-gradient(145deg, hsl(${hue} 58% 46%), hsl(${(hue + 36) % 360} 52% 36%))`
		: undefined}
>
	{#if photoSrc}
		<img src={photoSrc} alt="" class="absolute inset-0 size-full object-cover" draggable="false" />
	{:else}
		<span
			class={cn(
				'relative z-[1] block px-0.5 text-center font-semibold tracking-tight',
				primary ? 'text-primary-foreground' : 'text-white',
				initials.length > 1 ? 'text-[12px] leading-none' : 'text-[14px] leading-none',
				textClass
			)}
			style={primary ? undefined : 'color:#ffffff'}
		>
			{initials}
		</span>
	{/if}
</div>
