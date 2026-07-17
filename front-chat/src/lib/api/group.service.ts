import type { GroupInfo, GroupMember, GroupMembersResponse } from '$lib/chat/types';
import { request, requestForm } from './client';

/** Durable group create / join / leave / dissolve REST API. */
export const groupService = {
	create(opts?: { name?: string; group_id?: string }): Promise<GroupInfo> {
		return request<GroupInfo>('/api/groups', {
			method: 'POST',
			body: JSON.stringify(opts ?? {})
		});
	},

	listMine(): Promise<{ groups: GroupInfo[]; count: number }> {
		return request('/api/groups');
	},

	/**
	 * Fuzzy search groups by id / name (join autocomplete).
	 * GET /api/groups/search?q=&limit=20
	 */
	search(q: string, limit = 20): Promise<{ groups: GroupInfo[]; count: number; q?: string }> {
		const params = new URLSearchParams();
		if (q.trim()) params.set('q', q.trim());
		params.set('limit', String(limit));
		return request(`/api/groups/search?${params}`);
	},

	get(groupId: string): Promise<GroupInfo> {
		return request(`/api/groups/${encodeURIComponent(groupId)}`);
	},

	/** Owner-only: dissolve group and kick all members. */
	dissolve(groupId: string): Promise<{ group_id: string; name: string }> {
		return request(`/api/groups/${encodeURIComponent(groupId)}/dissolve`, { method: 'POST' });
	},

	/** Owner or admin: rename group. PATCH /api/groups/:id { name } */
	update(groupId: string, body: { name: string }): Promise<GroupInfo> {
		return request<GroupInfo>(`/api/groups/${encodeURIComponent(groupId)}`, {
			method: 'PATCH',
			body: JSON.stringify(body)
		});
	},

	/**
	 * Owner-only: promote/demote member.
	 * PATCH /api/groups/:id/members/:user_id { role: admin|member }
	 */
	setMemberRole(
		groupId: string,
		userId: string,
		role: 'admin' | 'member'
	): Promise<GroupMember> {
		return request<GroupMember>(
			`/api/groups/${encodeURIComponent(groupId)}/members/${encodeURIComponent(userId)}`,
			{
				method: 'PATCH',
				body: JSON.stringify({ role })
			}
		);
	},

	/**
	 * Join a group (must already exist).
	 * rejoin=true: restore membership after reconnect — no "加入到群" broadcast.
	 */
	join(groupId: string, opts?: { rejoin?: boolean }): Promise<unknown> {
		const q = new URLSearchParams({ group_id: groupId });
		if (opts?.rejoin) q.set('rejoin', '1');
		return request(`/api/groups/join?${q}`, { method: 'POST' });
	},

	leave(groupId: string, opts?: { silent?: boolean }): Promise<unknown> {
		const q = new URLSearchParams({ group_id: groupId });
		if (opts?.silent) q.set('silent', '1');
		return request(`/api/groups/leave?${q}`, { method: 'POST' });
	},

	members(groupId: string): Promise<GroupMembersResponse> {
		return request<GroupMembersResponse>(`/api/groups/${encodeURIComponent(groupId)}/members`);
	},

	/** Owner or admin: upload group icon (multipart field "file"). */
	uploadAvatar(
		groupId: string,
		file: File
	): Promise<{ group: GroupInfo; avatar: string; avatar_rev: number; url: string }> {
		const form = new FormData();
		form.append('file', file);
		return requestForm(`/api/groups/${encodeURIComponent(groupId)}/avatar`, form);
	}
};

/** Build group avatar URL with cache-bust rev. */
export function groupAvatarUrl(groupId: string, rev?: number): string {
	if (!groupId) return '';
	const base = `/api/group-avatar/${encodeURIComponent(groupId)}`;
	return rev && rev > 0 ? `${base}?v=${rev}` : base;
}
