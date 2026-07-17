import type { BlacklistUser, FriendRequest, FriendUser } from '$lib/chat/types';
import { request } from './client';

/** Friend invite / accept / remove / blacklist REST API. */
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

	/** 解除好友关系 */
	remove(userId: string): Promise<void> {
		return request(`/api/friends/${encodeURIComponent(userId)}`, { method: 'DELETE' });
	},

	/** 黑名单列表 */
	listBlacklist(): Promise<{ blacklist: BlacklistUser[]; count: number }> {
		return request('/api/friends/blacklist');
	},

	/** 拉黑（同时解除好友） */
	block(opts: { username?: string; user_id?: string }): Promise<BlacklistUser> {
		return request('/api/friends/blacklist', {
			method: 'POST',
			body: JSON.stringify(opts)
		});
	},

	/** 取消拉黑 */
	unblock(userId: string): Promise<void> {
		return request(`/api/friends/blacklist/${encodeURIComponent(userId)}`, {
			method: 'DELETE'
		});
	}
};
