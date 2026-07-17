import type { GroupMember, OnlineUser } from '../types';

export function withoutSelf(list: OnlineUser[], myUserId: string): OnlineUser[] {
	return list.filter((u) => u.user_id !== myUserId);
}

export function normalizeOnlineList(raw: unknown): OnlineUser[] {
	if (!Array.isArray(raw)) return [];
	const out: OnlineUser[] = [];
	for (const item of raw) {
		if (!item || typeof item !== 'object') continue;
		const o = item as Record<string, unknown>;
		const uid = String(o.user_id ?? o.id ?? '');
		if (!uid) continue;
		const name = String(o.username ?? o.name ?? uid);
		out.push({ user_id: uid, username: name || uid });
	}
	return out;
}

export function normalizeGroupMembers(raw: unknown): GroupMember[] {
	if (!Array.isArray(raw)) return [];
	const out: GroupMember[] = [];
	for (const item of raw) {
		if (!item || typeof item !== 'object') continue;
		const o = item as Record<string, unknown>;
		const uid = String(o.user_id ?? o.id ?? '');
		if (!uid) continue;
		const name = String(o.username ?? o.name ?? uid);
		const roleRaw = String(o.role ?? 'member').toLowerCase();
		const role = roleRaw === 'owner' ? 'owner' : roleRaw === 'admin' ? 'admin' : 'member';
		const online = o.online === true || o.online === 1 || o.online === 'true';
		out.push({
			user_id: uid,
			username: name || uid,
			role,
			online
		});
	}
	// owner > admin > member, then online, then name
	const rank = (r: string) => (r === 'owner' ? 0 : r === 'admin' ? 1 : 2);
	out.sort((a, b) => {
		const rd = rank(a.role) - rank(b.role);
		if (rd !== 0) return rd;
		if (a.online !== b.online) return a.online ? -1 : 1;
		return (a.username || '').localeCompare(b.username || '', 'zh');
	});
	return out;
}

export function formatTypingLabel(list: { username: string; user_id: string }[]): string {
	if (!list.length) return '';
	const names = list.map((u) => u.username || u.user_id).filter(Boolean);
	if (names.length === 1) return `${names[0]} 正在输入…`;
	if (names.length === 2) return `${names[0]}、${names[1]} 正在输入…`;
	return `${names[0]} 等 ${names.length} 人正在输入…`;
}
