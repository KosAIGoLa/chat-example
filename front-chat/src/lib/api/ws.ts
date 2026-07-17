/** WebSocket URL helpers (not REST). */

export function buildWsUrl(token: string): string {
	const base = import.meta.env.VITE_WS_BASE ?? window.location.origin;
	const wsBase = base.replace(/^http/, 'ws');
	return `${wsBase}/ws?token=${encodeURIComponent(token)}`;
}
