<script lang="ts">
	import { onDestroy } from 'svelte';
	import type { ChatMode } from '../types';
	import { formatDuration } from '../utils';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import Send from '@lucide/svelte/icons/send';
	import Mic from '@lucide/svelte/icons/mic';
	import Square from '@lucide/svelte/icons/square';
	import X from '@lucide/svelte/icons/x';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import Info from '@lucide/svelte/icons/info';

	const MAX_RECORD_SEC = 60;

	interface Props {
		chatMode: ChatMode;
		targetUser: string;
		groupId: string;
		value?: string;
		onSend: () => void;
		onSendVoice: (blob: Blob, durationSec: number) => Promise<void>;
	}

	let {
		chatMode,
		targetUser,
		groupId,
		value = $bindable(''),
		onSend,
		onSendVoice
	}: Props = $props();

	let recording = $state(false);
	let uploading = $state(false);
	let recordSeconds = $state(0);
	let errorMsg = $state('');

	let mediaRecorder: MediaRecorder | null = null;
	let mediaStream: MediaStream | null = null;
	let chunks: BlobPart[] = [];
	let startedAt = 0;
	let tickTimer: ReturnType<typeof setInterval> | null = null;
	let maxTimer: ReturnType<typeof setTimeout> | null = null;

	const canSend = $derived(
		(chatMode === 'private' && !!targetUser) || (chatMode === 'group' && !!groupId)
	);

	const hint = $derived(
		chatMode === 'private'
			? 'Select a target user to send private messages'
			: 'Join or select a group to send group messages'
	);

	const peerLabel = $derived(
		chatMode === 'private' ? `DM · ${targetUser}` : `Group · #${groupId}`
	);

	function handleSubmit(e: Event) {
		e.preventDefault();
		if (recording || uploading) return;
		onSend();
	}

	function pickMimeType(): string | undefined {
		const candidates = [
			'audio/webm;codecs=opus',
			'audio/webm',
			'audio/ogg;codecs=opus',
			'audio/mp4',
			'audio/mpeg'
		];
		if (typeof MediaRecorder === 'undefined') return undefined;
		for (const t of candidates) {
			if (MediaRecorder.isTypeSupported(t)) return t;
		}
		return undefined;
	}

	function clearTimers() {
		if (tickTimer) {
			clearInterval(tickTimer);
			tickTimer = null;
		}
		if (maxTimer) {
			clearTimeout(maxTimer);
			maxTimer = null;
		}
	}

	function stopTracks() {
		mediaStream?.getTracks().forEach((t) => t.stop());
		mediaStream = null;
	}

	async function startRecording() {
		errorMsg = '';
		if (!canSend || uploading || recording) return;
		if (!navigator.mediaDevices?.getUserMedia) {
			errorMsg = 'Microphone not supported in this browser';
			return;
		}

		try {
			mediaStream = await navigator.mediaDevices.getUserMedia({ audio: true });
		} catch {
			errorMsg = 'Microphone permission denied';
			return;
		}

		chunks = [];
		const mime = pickMimeType();
		try {
			mediaRecorder = mime
				? new MediaRecorder(mediaStream, { mimeType: mime })
				: new MediaRecorder(mediaStream);
		} catch {
			stopTracks();
			errorMsg = 'Failed to start recorder';
			return;
		}

		mediaRecorder.ondataavailable = (e) => {
			if (e.data.size > 0) chunks.push(e.data);
		};

		mediaRecorder.onstop = () => {
			const durationSec = Math.max(0.1, (Date.now() - startedAt) / 1000);
			// Prefer audio/* — some browsers report video/webm for audio-only.
			let type = mediaRecorder?.mimeType || mime || 'audio/webm';
			if (type.startsWith('video/webm')) type = 'audio/webm';
			if (type.startsWith('video/mp4')) type = 'audio/mp4';
			const blob = new Blob(chunks, { type });
			chunks = [];
			stopTracks();
			mediaRecorder = null;
			recording = false;
			clearTimers();
			void finishAndSend(blob, durationSec);
		};

		startedAt = Date.now();
		recordSeconds = 0;
		recording = true;
		mediaRecorder.start(200);

		tickTimer = setInterval(() => {
			recordSeconds = Math.floor((Date.now() - startedAt) / 1000);
		}, 250);

		maxTimer = setTimeout(() => {
			if (recording) stopRecording();
		}, MAX_RECORD_SEC * 1000);
	}

	function stopRecording() {
		if (!mediaRecorder || mediaRecorder.state === 'inactive') {
			recording = false;
			clearTimers();
			stopTracks();
			return;
		}
		mediaRecorder.stop();
	}

	function cancelRecording() {
		errorMsg = '';
		if (mediaRecorder && mediaRecorder.state !== 'inactive') {
			// Drop data by clearing chunks before stop handler runs.
			mediaRecorder.ondataavailable = null;
			mediaRecorder.onstop = () => {
				chunks = [];
				stopTracks();
				mediaRecorder = null;
				recording = false;
				clearTimers();
			};
			mediaRecorder.stop();
		} else {
			recording = false;
			clearTimers();
			stopTracks();
		}
	}

	async function finishAndSend(blob: Blob, durationSec: number) {
		// Real WebM/Opus frames are tiny; keep a low floor but allow short notes.
		if (blob.size < 32) {
			errorMsg = 'Recording too short — hold a bit longer';
			return;
		}
		uploading = true;
		errorMsg = '';
		try {
			await onSendVoice(blob, durationSec);
		} catch (err) {
			errorMsg = (err as Error).message || 'Failed to send voice';
			console.error('[voice] send failed', err, { size: blob.size, type: blob.type, durationSec });
		} finally {
			uploading = false;
			recordSeconds = 0;
		}
	}

	onDestroy(() => {
		cancelRecording();
	});
</script>

<div class="bg-background border-t p-3 md:p-4">
	{#if !canSend}
		<div
			class="text-muted-foreground bg-muted/40 flex items-center gap-2 rounded-lg border border-dashed px-4 py-3 text-sm"
		>
			<Info class="size-4 shrink-0" />
			{hint}
		</div>
	{:else if recording}
		<div class="flex items-center gap-3">
			<div
				class="bg-destructive/10 text-destructive flex min-w-0 flex-1 items-center gap-3 rounded-lg border border-destructive/20 px-4 py-2.5"
			>
				<span class="relative flex size-2.5 shrink-0">
					<span
						class="bg-destructive absolute inline-flex size-full animate-ping rounded-full opacity-60"
					></span>
					<span class="bg-destructive relative inline-flex size-2.5 rounded-full"></span>
				</span>
				<div class="min-w-0 flex-1">
					<p class="text-sm font-medium">Recording…</p>
					<p class="text-xs opacity-80">
						{formatDuration(recordSeconds)} / {formatDuration(MAX_RECORD_SEC)}
					</p>
				</div>
			</div>
			<Button
				type="button"
				variant="outline"
				size="lg"
				class="h-11 shrink-0"
				onclick={cancelRecording}
				aria-label="Cancel recording"
			>
				<X class="size-4" />
			</Button>
			<Button
				type="button"
				size="lg"
				class="h-11 shrink-0"
				onclick={stopRecording}
				aria-label="Stop and send"
			>
				<Square class="size-4 fill-current" />
				Send
			</Button>
		</div>
	{:else}
		<form onsubmit={handleSubmit} class="flex items-end gap-2">
			<div class="min-w-0 flex-1 space-y-1.5">
				<p class="text-muted-foreground px-1 text-[11px] font-medium tracking-wide uppercase">
					{peerLabel}
				</p>
				{#if errorMsg}
					<p class="text-destructive px-1 text-xs">{errorMsg}</p>
				{/if}
				<Input
					bind:value
					placeholder="Type a message…"
					class="h-11"
					autocomplete="off"
					disabled={uploading}
				/>
			</div>
			<Button
				type="button"
				variant="outline"
				size="lg"
				class="h-11 shrink-0"
				disabled={uploading}
				onclick={startRecording}
				aria-label="Record voice message"
				title="Voice message"
			>
				{#if uploading}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					<Mic class="size-4" />
				{/if}
			</Button>
			<Button
				type="submit"
				size="lg"
				class="h-11 shrink-0"
				disabled={!value.trim() || uploading}
			>
				<Send class="size-4" />
				Send
			</Button>
		</form>
	{/if}
</div>
