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
		/** Remote users typing label, e.g. "Alice 正在打字…" */
		typingHint?: string;
		onSend: () => void;
		onSendVoice: (blob: Blob, durationSec: number) => Promise<void>;
		onOpenRedPacket?: () => void;
		/** Called when local user types in the input (throttled by parent). */
		onTyping?: () => void;
	}

	let {
		chatMode,
		targetUser,
		groupId,
		value = $bindable(''),
		typingHint = '',
		onSend,
		onSendVoice,
		onOpenRedPacket,
		onTyping
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
		chatMode === 'private' ? '选择好友后发送私聊、语音或红包' : '选择群聊后发送消息、语音或拼手气红包'
	);

	const peerLabel = $derived(
		chatMode === 'private' ? `私聊 · ${targetUser}` : `群 · #${groupId}`
	);

	function handleSubmit(e: Event) {
		e.preventDefault();
		if (recording || uploading) return;
		onSend();
	}

	function handleInput() {
		// Fire for every input change including Chinese IME composition updates.
		onTyping?.();
	}

	function handleKeydown(e: KeyboardEvent) {
		// Extra path for keys that may not always emit input the same way.
		if (e.key === 'Process') return;
		if (e.key.length === 1 || e.key === 'Backspace' || e.key === 'Delete' || e.key === 'Enter') {
			onTyping?.();
		}
	}

	function handleCompositionUpdate() {
		// Chinese / Japanese IME: keep broadcasting while composing.
		onTyping?.();
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
	{#if typingHint}
		<div
			class="mb-2 flex items-center gap-2 rounded-lg bg-emerald-500/10 px-3 py-1.5 text-xs text-emerald-700 dark:text-emerald-300"
		>
			<span class="inline-flex gap-0.5">
				<span class="size-1.5 animate-bounce rounded-full bg-emerald-500 [animation-delay:0ms]"
				></span>
				<span
					class="size-1.5 animate-bounce rounded-full bg-emerald-500 [animation-delay:120ms]"
				></span>
				<span
					class="size-1.5 animate-bounce rounded-full bg-emerald-500 [animation-delay:240ms]"
				></span>
			</span>
			<span class="truncate font-medium">{typingHint}</span>
		</div>
	{/if}
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
					placeholder="输入消息…"
					class="h-11"
					autocomplete="off"
					disabled={uploading}
					oninput={handleInput}
					onkeydown={handleKeydown}
					oncompositionstart={handleCompositionUpdate}
					oncompositionupdate={handleCompositionUpdate}
					oncompositionend={handleCompositionUpdate}
				/>
			</div>
			{#if onOpenRedPacket}
				<Button
					type="button"
					size="lg"
					class="h-11 shrink-0 border-0 bg-gradient-to-br from-[#f04a3a] to-[#c02218] text-white shadow-sm hover:brightness-110"
					disabled={uploading}
					onclick={onOpenRedPacket}
					aria-label="发红包"
					title="发红包"
				>
					<span
						class="flex size-5 items-center justify-center rounded-full bg-gradient-to-br from-[#ffe9a8] to-[#d4a017] text-[10px] font-bold text-[#8b1a12]"
						style="font-family: Songti SC, STSong, serif;">福</span
					>
					<span class="hidden sm:inline">红包</span>
				</Button>
			{/if}
			<Button
				type="button"
				variant="outline"
				size="lg"
				class="h-11 shrink-0"
				disabled={uploading}
				onclick={startRecording}
				aria-label="Record voice message"
				title="语音消息"
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
				发送
			</Button>
		</form>
	{/if}
</div>
