import type { FriendRequest, FriendUser } from '$lib/chat/types';
import { request } from './client';

/** Friend invite / accept / list REST API. */
export const friendService = {
	listFriends(): Promise<{ friends: FriendUser[]; count: number }> {
		return request('/api/friends');
	},

	listIncoming(): Promise<{ requests: FriendRequest[]; count: number }> {
		return request('/api/friends/requests/incoming');
	},

	listOutgoing(): Promise<{ requests: FriendRequest[]; count: number }> {
		return request('/api/friends/requests/outgoing');
	},

	/** Invite by username (preferred) or user_id. Pending until the other accepts. */
	invite(opts: { username?: string; user_id?: string }): Promise<FriendRequest> {
		return request('/api/friends/request', {
			method: 'POST',
			body: JSON.stringify(opts)
		});
	},

	accept(id: number): Promise<FriendRequest> {
		return request(`/api/friends/requests/${id}/accept`, { method: 'POST' });
	},

	reject(id: number): Promise<FriendRequest> {
		return request(`/api/friends/requests/${id}/reject`, { method: 'POST' });
	},

	remove(userId: string): Promise<void> {
		return request(`/api/friends/${encodeURIComponent(userId)}`, { method: 'DELETE' });
	}
};
