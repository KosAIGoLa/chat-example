/**
 * Shared HTTP client for all REST services.
 * - Injects Bearer token from localStorage
 * - Unwraps APIResponseDTO { code, message, data }
 */

import type { APIResponse } from '$lib/types';

export const API_BASE = import.meta.env.VITE_API_BASE ?? '';

export class ApiError extends Error {
	readonly code: number;

	constructor(code: number, message: string) {
		super(message);
		this.name = 'ApiError';
		this.code = code;
	}
}

function authHeaders(extra?: HeadersInit): Record<string, string> {
	const headers: Record<string, string> = {
		...(extra as Record<string, string> | undefined)
	};
	const token = typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null;
	if (token) {
		headers['Authorization'] = `Bearer ${token}`;
	}
	return headers;
}

/** JSON request helper — sets Content-Type and Authorization. */
export async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
	const headers = authHeaders({
		'Content-Type': 'application/json',
		...((options.headers as Record<string, string>) ?? {})
	});

	const res = await fetch(`${API_BASE}${path}`, { ...options, headers });
	let body: APIResponse<T>;
	try {
		body = await res.json();
	} catch {
		throw new ApiError(res.status || 500, res.ok ? 'Invalid JSON response' : `HTTP ${res.status}`);
	}

	if (body.code >= 400) {
		throw new ApiError(body.code, body.message || 'Request failed');
	}
	return body.data as T;
}

/**
 * Multipart / binary-friendly request (e.g. voice upload).
 * Does not force Content-Type so the browser sets multipart boundary.
 */
export async function requestForm<T>(path: string, form: FormData, method = 'POST'): Promise<T> {
	const token = typeof localStorage !== 'undefined' ? localStorage.getItem('token') : null;
	if (!token) {
		throw new ApiError(401, 'Not logged in');
	}

	const res = await fetch(`${API_BASE}${path}`, {
		method,
		headers: { Authorization: `Bearer ${token}` },
		body: form
	});

	let body: APIResponse<T>;
	try {
		body = await res.json();
	} catch {
		throw new ApiError(
			res.status || 500,
			res.ok ? 'Invalid upload response' : `Upload failed (HTTP ${res.status})`
		);
	}
	if (!res.ok || body.code >= 400) {
		throw new ApiError(body.code || res.status, body.message || `Upload failed (HTTP ${res.status})`);
	}
	return body.data as T;
}

/** Build absolute URL with optional query token for &lt;audio src&gt; / media. */
export function buildAuthedUrl(path: string): string {
	if (!path) return '';
	if (path.startsWith('http://') || path.startsWith('https://') || path.startsWith('blob:')) {
		return path;
	}
	const token = typeof localStorage !== 'undefined' ? (localStorage.getItem('token') ?? '') : '';
	const base = API_BASE || '';
	const sep = path.includes('?') ? '&' : '?';
	return `${base}${path}${sep}token=${encodeURIComponent(token)}`;
}
