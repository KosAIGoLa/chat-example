import type { CallMedia } from '$lib/chat/types';
import { request } from './client';

export type CallType = 'private' | 'group';

export interface LiveKitTokenResponse {
	token: string;
	url: string;
	room: string;
	identity: string;
	call_type: CallType;
	peer_id?: string;
	group_id?: string;
	media?: CallMedia | string;
}

export interface CallSignalPayload {
	action: 'invite' | 'accept' | 'reject' | 'end' | 'cancel';
	room: string;
	call_type: CallType;
	/** audio = 语音, video = 视讯 */
	media?: CallMedia;
	to?: string;
	group_id?: string;
	from_name?: string;
}

/** Group meeting snapshot from GET/POST /api/livekit/meeting. */
export interface MeetingStatus {
	active: boolean;
	group_id?: string;
	room?: string;
	media?: CallMedia | string;
	started_by?: string;
	started_by_name?: string;
	started_at?: number;
	participant_count: number;
	/** Present after start/join. */
	token?: string;
	url?: string;
	identity?: string;
	created?: boolean;
	ended?: boolean;
}

/** LiveKit token + private call signal + group meeting REST API. */
export const livekitService = {
	/** Mint a room token (private friend call; group prefers meeting API). */
	createToken(body: {
		type: CallType;
		peer_id?: string;
		group_id?: string;
		room?: string;
	}): Promise<LiveKitTokenResponse> {
		return request<LiveKitTokenResponse>('/api/livekit/token', {
			method: 'POST',
			body: JSON.stringify(body)
		});
	},

	/** Relay private invite / accept / reject / end over the chat WebSocket hub. */
	signal(body: CallSignalPayload): Promise<unknown> {
		return request('/api/livekit/signal', {
			method: 'POST',
			body: JSON.stringify(body)
		});
	},

	/**
	 * Group conference mode (not a private ring-call):
	 * start | join | leave | end
	 */
	meeting(body: {
		group_id: string;
		action: 'start' | 'join' | 'leave' | 'end';
		media?: CallMedia;
	}): Promise<MeetingStatus> {
		return request<MeetingStatus>('/api/livekit/meeting', {
			method: 'POST',
			body: JSON.stringify(body)
		});
	},

	/** Whether a group currently has an open meeting. */
	getMeeting(groupId: string): Promise<MeetingStatus> {
		return request<MeetingStatus>(`/api/livekit/meeting/${encodeURIComponent(groupId)}`);
	}
};
