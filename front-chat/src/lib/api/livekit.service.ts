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

/** LiveKit token + call signaling REST API. */
export const livekitService = {
	/** Mint a room token (private friend call or group meeting). */
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

	/** Relay invite / accept / reject / end over the chat WebSocket hub. */
	signal(body: CallSignalPayload): Promise<unknown> {
		return request('/api/livekit/signal', {
			method: 'POST',
			body: JSON.stringify(body)
		});
	}
};
