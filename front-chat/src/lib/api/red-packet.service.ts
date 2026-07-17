import { request } from './client';

export interface RedPacketClaim {
	user_id: string;
	username: string;
	amount: number;
	created_at: number;
}

export interface RedPacket {
	id: string;
	type: 'private' | 'group' | string;
	from_user_id: string;
	to_user_id?: string;
	group_id?: string;
	total_amount: number;
	total_count: number;
	remaining_amount: number;
	remaining_count: number;
	greeting: string;
	status: string;
	message_id?: string;
	expires_at: number;
	created_at: number;
	my_claim_amount?: number;
	claims?: RedPacketClaim[];
}

export interface ClaimResult {
	packet_id: string;
	amount: number;
	remaining_count: number;
	finished: boolean;
	balance: number;
	status: string;
}

export interface WalletInfo {
	balance: number;
}

export interface CreateRedPacketBody {
	type: 'private' | 'group';
	peer_id?: string;
	group_id?: string;
	total_amount: number;
	total_count?: number;
	greeting?: string;
}

export interface CreateRedPacketResult {
	packet: RedPacket;
	message: {
		id?: string;
		type: string;
		from: string;
		to: string;
		content: string;
		group_id?: string;
		timestamp?: number;
		content_type?: string;
		red_packet_id?: string;
	};
}

export const redPacketService = {
	getWallet(): Promise<WalletInfo> {
		return request<WalletInfo>('/api/wallet/me');
	},

	create(body: CreateRedPacketBody): Promise<CreateRedPacketResult> {
		return request<CreateRedPacketResult>('/api/red-packets', {
			method: 'POST',
			body: JSON.stringify(body)
		});
	},

	get(id: string): Promise<RedPacket> {
		return request<RedPacket>(`/api/red-packets/${encodeURIComponent(id)}`);
	},

	claim(id: string): Promise<ClaimResult> {
		return request<ClaimResult>(`/api/red-packets/${encodeURIComponent(id)}/claim`, {
			method: 'POST',
			body: JSON.stringify({})
		});
	}
};
