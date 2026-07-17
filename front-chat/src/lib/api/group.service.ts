import type { GroupInfo, GroupMembersResponse } from '$lib/chat/types';
import { request } from './client';

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

	get(groupId: string): Promise<GroupInfo> {
		return request(`/api/groups/${encodeURIComponent(groupId)}`);
	},

	/** Owner-only: dissolve group and kick all members. */
	dissolve(groupId: string): Promise<{ group_id: string; name: string }> {
		return request(`/api/groups/${encodeURIComponent(groupId)}/dissolve`, { method: 'POST' });
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
	}
};
