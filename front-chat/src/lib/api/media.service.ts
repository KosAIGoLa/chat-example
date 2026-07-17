import type { VoiceUploadResult } from '$lib/chat/types';
import { API_BASE, buildAuthedUrl, requestForm } from './client';

/** Fix MediaRecorder MIME labels so the server accepts the file. */
function normalizeVoiceBlob(blob: Blob): Blob {
	const raw = (blob.type || '').split(';')[0].trim().toLowerCase();
	let type = raw;
	if (!type || type === 'application/octet-stream') {
		type = 'audio/webm';
	} else if (type === 'video/webm') {
		// Audio-only WebM from MediaRecorder.
		type = 'audio/webm';
	} else if (type === 'video/mp4') {
		type = 'audio/mp4';
	}
	if (type === blob.type) return blob;
	return new Blob([blob], { type });
}

function guessAudioExt(mime: string): string {
	const m = (mime || '').split(';')[0].trim().toLowerCase();
	if (m.includes('webm')) return '.webm';
	if (m.includes('ogg')) return '.ogg';
	if (m.includes('mp4') || m.includes('m4a') || m.includes('aac')) return '.m4a';
	if (m.includes('mpeg') || m.includes('mp3')) return '.mp3';
	if (m.includes('wav')) return '.wav';
	return '.webm';
}

/** Voice / media REST API. */
export const mediaService = {
	/** Upload a recorded voice blob. */
	async uploadVoice(blob: Blob, durationSec: number): Promise<VoiceUploadResult> {
		const fixed = normalizeVoiceBlob(blob);
		const form = new FormData();
		const ext = guessAudioExt(fixed.type);
		form.append('file', fixed, `voice${ext}`);
		form.append('duration', String(Math.max(0, durationSec)));

		const data = await requestForm<VoiceUploadResult>('/api/voice', form);
		if (!data?.url) {
			throw new Error('Upload succeeded but no media URL returned');
		}
		return data;
	},

	/** Build an authenticated media URL suitable for &lt;audio src&gt;. */
	buildMediaUrl(path: string): string {
		return buildAuthedUrl(path);
	},

	/** Expose base for rare callers that need absolute paths. */
	get base(): string {
		return API_BASE;
	}
};

/** @deprecated Prefer mediaService.buildMediaUrl */
export const buildMediaUrl = (path: string) => mediaService.buildMediaUrl(path);
