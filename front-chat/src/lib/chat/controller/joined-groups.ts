import { JOINED_GROUPS_KEY } from './constants';

export function loadJoinedGroups(): string[] {
	if (typeof window === 'undefined') return [];
	try {
		const raw = localStorage.getItem(JOINED_GROUPS_KEY);
		if (!raw) return [];
		const parsed = JSON.parse(raw);
		return Array.isArray(parsed) ? parsed.filter((g) => typeof g === 'string') : [];
	} catch {
		return [];
	}
}

export function saveJoinedGroups(groups: string[]) {
	if (typeof window === 'undefined') return;
	localStorage.setItem(JOINED_GROUPS_KEY, JSON.stringify(groups));
}

/** Append unique string ids without mutating via Set (eslint/svelte reactivity). */
export function appendUnique(list: string[], id: string): string[] {
	return list.includes(id) ? list : [...list, id];
}
