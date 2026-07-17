/**
 * Lightweight phone tones via Web Audio API (no audio asset files).
 * - ringback: caller hears while waiting
 * - ringtone: callee hears on incoming invite
 * - connect: short confirm when both joined
 * - end: short hang-up beep
 */

type ToneMode = 'ringback' | 'ringtone' | 'connect' | 'end';

let ctx: AudioContext | null = null;
let timer: ReturnType<typeof setInterval> | null = null;
let activeMode: ToneMode | null = null;

function getCtx(): AudioContext {
	if (!ctx) {
		const AC =
			window.AudioContext ||
			(window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext;
		ctx = new AC();
	}
	return ctx;
}

function stopOscillators(nodes: AudioNode[]) {
	for (const n of nodes) {
		try {
			if ('stop' in n && typeof (n as OscillatorNode).stop === 'function') {
				(n as OscillatorNode).stop();
			}
			n.disconnect();
		} catch {
			// ignore
		}
	}
}

/** Dual-tone burst (classic phone-ish). */
function playBurst(
	audio: AudioContext,
	freqs: number[],
	durationMs: number,
	gain = 0.08
): AudioNode[] {
	const now = audio.currentTime;
	const g = audio.createGain();
	g.gain.value = 0;
	g.connect(audio.destination);
	// Attack / release envelope
	g.gain.linearRampToValueAtTime(gain, now + 0.02);
	g.gain.linearRampToValueAtTime(gain, now + durationMs / 1000 - 0.05);
	g.gain.linearRampToValueAtTime(0, now + durationMs / 1000);

	const nodes: AudioNode[] = [g];
	for (const f of freqs) {
		const o = audio.createOscillator();
		o.type = 'sine';
		o.frequency.value = f;
		o.connect(g);
		o.start(now);
		o.stop(now + durationMs / 1000 + 0.02);
		nodes.push(o);
	}
	return nodes;
}

function clearTimer() {
	if (timer != null) {
		clearInterval(timer);
		timer = null;
	}
}

export function stopCallSounds() {
	clearTimer();
	activeMode = null;
	// Soft-close context gain by suspending (keeps ctx reusable).
	if (ctx && ctx.state === 'running') {
		void ctx.suspend().catch(() => undefined);
	}
}

async function ensureRunning() {
	const audio = getCtx();
	if (audio.state === 'suspended') {
		await audio.resume();
	}
	return audio;
}

/** Caller: ringback every ~3s (US-style 2s on / 4s cycle simplified to 1s on / 2s off). */
export async function startRingback() {
	stopCallSounds();
	activeMode = 'ringback';
	const audio = await ensureRunning();
	const tick = () => {
		if (activeMode !== 'ringback') return;
		playBurst(audio, [440, 480], 1000, 0.06);
	};
	tick();
	timer = setInterval(tick, 3000);
}

/** Callee: ringtone every ~2.5s (higher dual tone). */
export async function startRingtone() {
	stopCallSounds();
	activeMode = 'ringtone';
	const audio = await ensureRunning();
	const tick = () => {
		if (activeMode !== 'ringtone') return;
		// Two short rings
		playBurst(audio, [480, 620], 350, 0.09);
		setTimeout(() => {
			if (activeMode === 'ringtone') playBurst(audio, [480, 620], 350, 0.09);
		}, 450);
	};
	tick();
	timer = setInterval(tick, 2500);
}

export async function playConnectTone() {
	stopCallSounds();
	const audio = await ensureRunning();
	playBurst(audio, [523.25, 659.25], 180, 0.07); // C5 + E5
	setTimeout(() => playBurst(audio, [783.99], 220, 0.06), 160); // G5
}

export async function playEndTone() {
	stopCallSounds();
	const audio = await ensureRunning();
	playBurst(audio, [400, 300], 250, 0.07);
}
