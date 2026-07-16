/**
 * AES-256-GCM helpers for:
 *  1) Chat content field: enc:v1:<base64(nonce||ciphertext||tag)>
 *  2) Full WebSocket frames: {"type":"ws_enc","v":1,"data":"<base64…>"}
 */

const ENC_PREFIX = 'enc:v1:';
const WS_ENC_TYPE = 'ws_enc';

let cachedKey: CryptoKey | null = null;
let cachedKeyB64: string | null = null;

export function isEncryptedContent(content: string | undefined | null): boolean {
	return typeof content === 'string' && content.startsWith(ENC_PREFIX);
}

export function isWSEncryptedFrame(raw: string): boolean {
	const s = raw.trim();
	return s.includes('"type":"ws_enc"') || s.includes('"type": "ws_enc"');
}

function b64ToBytes(b64: string): Uint8Array {
	const bin = atob(b64);
	const out = new Uint8Array(bin.length);
	for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
	return out;
}

function bytesToB64(bytes: Uint8Array): string {
	let bin = '';
	for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
	return btoa(bin);
}

/** Import raw AES key (base64 from GET /api/crypto/key). */
export async function importMessageKey(keyBase64: string): Promise<CryptoKey> {
	if (cachedKey && cachedKeyB64 === keyBase64) return cachedKey;
	const raw = b64ToBytes(keyBase64);
	if (raw.byteLength !== 32) {
		throw new Error(`Invalid message key length: ${raw.byteLength} (want 32)`);
	}
	// Copy into a fresh ArrayBuffer for Web Crypto typings.
	const buf = new ArrayBuffer(raw.byteLength);
	new Uint8Array(buf).set(raw);
	const key = await crypto.subtle.importKey('raw', buf, { name: 'AES-GCM' }, false, [
		'encrypt',
		'decrypt'
	]);
	cachedKey = key;
	cachedKeyB64 = keyBase64;
	return key;
}

export function clearMessageKey(): void {
	cachedKey = null;
	cachedKeyB64 = null;
}

export function hasMessageKey(): boolean {
	return cachedKey != null;
}

/** Encrypt plaintext → enc:v1:… */
export async function encryptContent(plaintext: string, key?: CryptoKey): Promise<string> {
	if (!plaintext) return '';
	if (isEncryptedContent(plaintext)) return plaintext;
	const k = key ?? cachedKey;
	if (!k) throw new Error('Message encryption key not loaded');

	const nonce = crypto.getRandomValues(new Uint8Array(12));
	const encoded = new TextEncoder().encode(plaintext);
	const sealed = new Uint8Array(
		await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce }, k, encoded)
	);
	// nonce || ciphertext+tag
	const out = new Uint8Array(nonce.length + sealed.length);
	out.set(nonce, 0);
	out.set(sealed, nonce.length);
	return ENC_PREFIX + bytesToB64(out);
}

/** Decrypt enc:v1:… → plaintext. Legacy plaintext returned as-is. */
export async function decryptContent(content: string, key?: CryptoKey): Promise<string> {
	if (!content) return '';
	if (!isEncryptedContent(content)) return content;
	const k = key ?? cachedKey;
	if (!k) throw new Error('Message encryption key not loaded');

	const raw = b64ToBytes(content.slice(ENC_PREFIX.length));
	if (raw.byteLength < 13) throw new Error('ciphertext too short');
	const nonce = raw.slice(0, 12);
	const sealed = raw.slice(12);
	// Ensure ArrayBuffer-backed views for subtle.decrypt
	const nonceBuf = new Uint8Array(nonce);
	const sealedBuf = new Uint8Array(sealed);
	const plain = await crypto.subtle.decrypt(
		{ name: 'AES-GCM', iv: nonceBuf },
		k,
		sealedBuf
	);
	return new TextDecoder().decode(plain);
}

/** Best-effort decrypt for display; never throws. */
export async function tryDecryptContent(content: string): Promise<string> {
	try {
		return await decryptContent(content);
	} catch {
		// Key missing or corrupt — show placeholder instead of raw base64.
		if (isEncryptedContent(content)) return '[encrypted message]';
		return content;
	}
}

/**
 * Seal an entire application JSON string for WebSocket transport.
 * Output: {"type":"ws_enc","v":1,"data":"…"}
 */
export async function sealWSFrame(plainJSON: string, key?: CryptoKey): Promise<string> {
	if (!plainJSON) return plainJSON;
	if (isWSEncryptedFrame(plainJSON)) return plainJSON;
	const k = key ?? cachedKey;
	if (!k) throw new Error('Message encryption key not loaded');

	const nonce = crypto.getRandomValues(new Uint8Array(12));
	const encoded = new TextEncoder().encode(plainJSON);
	const sealed = new Uint8Array(
		await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce }, k, encoded)
	);
	const out = new Uint8Array(nonce.length + sealed.length);
	out.set(nonce, 0);
	out.set(sealed, nonce.length);
	return JSON.stringify({
		type: WS_ENC_TYPE,
		v: 1,
		data: bytesToB64(out)
	});
}

/**
 * Open a WebSocket wire frame. Plain JSON (legacy) returned as-is.
 */
export async function openWSFrame(raw: string, key?: CryptoKey): Promise<string> {
	if (!raw) return raw;
	if (!isWSEncryptedFrame(raw)) return raw;
	const k = key ?? cachedKey;
	if (!k) throw new Error('Message encryption key not loaded');

	let env: { type?: string; data?: string };
	try {
		env = JSON.parse(raw) as { type?: string; data?: string };
	} catch {
		return raw;
	}
	if (env.type !== WS_ENC_TYPE || !env.data) {
		return raw;
	}
	const blob = b64ToBytes(env.data);
	if (blob.byteLength < 13) throw new Error('ws frame too short');
	const nonce = new Uint8Array(blob.slice(0, 12));
	const sealed = new Uint8Array(blob.slice(12));
	const plain = await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce }, k, sealed);
	return new TextDecoder().decode(plain);
}

/** Best-effort open; on failure returns empty string so caller can drop. */
export async function tryOpenWSFrame(raw: string): Promise<string> {
	try {
		return await openWSFrame(raw);
	} catch (err) {
		console.error('[crypto] open WS frame failed', err);
		return '';
	}
}
