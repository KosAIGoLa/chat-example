/**
 * Client API layer — domain services (preferred) + legacy `api` facade.
 *
 * Prefer:
 *   import { authService, friendService, groupService } from '$lib/api'
 *
 * Legacy (still works):
 *   import { api } from '$lib/api'
 */

export { API_BASE, ApiError, request, requestForm, buildAuthedUrl } from './client';
export { authService, avatarUrl } from './auth.service';
export type { AvatarUploadResult } from './auth.service';
export { friendService } from './friend.service';
export { groupService } from './group.service';
export { chatService } from './chat.service';
export { mediaService, buildMediaUrl } from './media.service';
export { livekitService } from './livekit.service';
export type {
	CallType,
	CallSignalPayload,
	LiveKitTokenResponse,
	MeetingStatus
} from './livekit.service';
export { redPacketService } from './red-packet.service';
export type {
	RedPacket,
	RedPacketClaim,
	ClaimResult,
	WalletInfo,
	CreateRedPacketBody,
	CreateRedPacketResult
} from './red-packet.service';
export { buildWsUrl } from './ws';

import { authService } from './auth.service';
import { chatService } from './chat.service';
import { friendService } from './friend.service';
import { groupService } from './group.service';
import { mediaService } from './media.service';

/**
 * Flat facade kept for older call sites.
 * New code should import the domain services above.
 */
export const api = {
	// auth
	register: authService.register,
	login: authService.login,
	getMe: authService.getMe,
	updateProfile: authService.updateProfile,

	// chat transport
	getCryptoKey: chatService.getCryptoKey,
	getOnlineUsers: chatService.getOnlineUsers,
	getPrivateHistory: chatService.getPrivateHistory,
	getGroupHistory: chatService.getGroupHistory,

	// friends
	listFriends: friendService.listFriends,
	listIncomingFriendRequests: friendService.listIncoming,
	listOutgoingFriendRequests: friendService.listOutgoing,
	inviteFriend: friendService.invite,
	acceptFriendRequest: friendService.accept,
	rejectFriendRequest: friendService.reject,
	removeFriend: friendService.remove,
	listBlacklist: friendService.listBlacklist,
	blockUser: friendService.block,
	unblockUser: friendService.unblock,

	// groups
	createGroup: groupService.create,
	listMyGroups: groupService.listMine,
	dissolveGroup: groupService.dissolve,
	joinGroup: groupService.join,
	leaveGroup: groupService.leave,
	getGroupMembers: groupService.members,

	// media
	uploadVoice: mediaService.uploadVoice
} as const;
