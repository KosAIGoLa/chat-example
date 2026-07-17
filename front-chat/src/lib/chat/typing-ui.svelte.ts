/**
 * Module-level typing hint so WebSocket async handlers can update UI reliably
 * (controller $state + parent callback is easy to miss across await boundaries).
 */
export const typingUI = $state({
	/** e.g. "bcd123 正在输入…" */
	hint: '',
	/** conversation key: private:6 | group:123456 */
	convKey: '',
	updatedAt: 0
});

export function setTypingHint(hint: string, convKey = '') {
	// Always assign both fields so Svelte tracks a full update.
	typingUI.hint = hint ?? '';
	typingUI.convKey = convKey || typingUI.convKey;
	typingUI.updatedAt = Date.now();
}

export function clearTypingHint() {
	typingUI.hint = '';
	typingUI.convKey = '';
	typingUI.updatedAt = Date.now();
}

export function activeConvKey(mode: string, peer: string, group: string): string {
	if (mode === 'group' && group) return `group:${group}`;
	if (mode === 'private' && peer) return `private:${peer}`;
	return '';
}
