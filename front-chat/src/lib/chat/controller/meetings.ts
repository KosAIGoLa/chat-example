/**
 * Group conference (meeting mode) state — distinct from private 1:1 calls.
 */

import { livekitService } from '$lib/api/livekit.service';
import type { ActiveGroupMeeting, MeetingEvent } from '../types';
import { toastInfo } from '$lib/ui/notify.svelte';

export interface MeetingsDeps {
	getMyUserId: () => string;
	getActiveMeetings: () => Record<string, ActiveGroupMeeting>;
	setActiveMeetings: (m: Record<string, ActiveGroupMeeting>) => void;
	groupDisplayName: (id: string) => string;
}

export function createMeetingsApi(deps: MeetingsDeps) {
	function applyMeetingEvent(ev: MeetingEvent) {
		const gid = (ev.group_id || '').trim();
		if (!gid) return;
		const activeMeetings = deps.getActiveMeetings();
		if (ev.action === 'ended') {
			const next = { ...activeMeetings };
			delete next[gid];
			deps.setActiveMeetings(next);
			return;
		}
		if (
			ev.action === 'started' ||
			ev.action === 'joined' ||
			ev.action === 'left' ||
			ev.action === 'snapshot'
		) {
			const media: 'audio' | 'video' = ev.media === 'video' ? 'video' : 'audio';
			const prev = activeMeetings[gid];
			deps.setActiveMeetings({
				...activeMeetings,
				[gid]: {
					group_id: gid,
					room: ev.room || prev?.room || `grp_${gid}`,
					media: media || prev?.media || 'audio',
					started_by: prev?.started_by || ev.from,
					started_by_name: prev?.started_by_name || ev.from_name || ev.from,
					started_at: prev?.started_at || ev.timestamp || Math.floor(Date.now() / 1000),
					participant_count:
						typeof ev.participant_count === 'number'
							? ev.participant_count
							: (prev?.participant_count ?? 1)
				}
			});
			// snapshot = silent catch-up (join_group / reconnect); do not toast.
			if (ev.action === 'started' && ev.from !== deps.getMyUserId()) {
				const gname = deps.groupDisplayName(gid);
				const kind = media === 'video' ? '视讯' : '语音';
				toastInfo(
					`${ev.from_name || ev.from} 开启了 #${gname} ${kind}会议`,
					'群会议'
				);
			}
		}
	}

	/** In-flight de-dupe so selectGroup + reconnect + visibility don't stampede. */
	const inflight = new Map<string, Promise<void>>();

	async function refreshGroupMeeting(gid: string) {
		const id = (gid || '').trim();
		if (!id) return;
		const existing = inflight.get(id);
		if (existing) return existing;
		const p = refreshGroupMeetingInner(id).finally(() => {
			if (inflight.get(id) === p) inflight.delete(id);
		});
		inflight.set(id, p);
		return p;
	}

	async function refreshGroupMeetingInner(id: string) {
		try {
			const st = await livekitService.getMeeting(id);
			const activeMeetings = deps.getActiveMeetings();
			if (!st || st.active !== true) {
				if (activeMeetings[id]) {
					const next = { ...activeMeetings };
					delete next[id];
					deps.setActiveMeetings(next);
				}
				return;
			}
			const snapshot: ActiveGroupMeeting = {
				group_id: id,
				room: (st.room && String(st.room)) || `grp_${id}`,
				media: st.media === 'video' ? 'video' : 'audio',
				started_by: st.started_by ? String(st.started_by) : '',
				started_by_name: String(st.started_by_name || st.started_by || ''),
				started_at: typeof st.started_at === 'number' ? st.started_at : 0,
				participant_count:
					typeof st.participant_count === 'number' ? st.participant_count : 0
			};
			deps.setActiveMeetings({ ...activeMeetings, [id]: snapshot });
		} catch {
			// ignore network / 401 — WS snapshot or next keypoint will retry
		}
	}

	function setActiveMeeting(gid: string, info: ActiveGroupMeeting | null) {
		const id = gid.trim();
		if (!id) return;
		const activeMeetings = deps.getActiveMeetings();
		if (!info) {
			const next = { ...activeMeetings };
			delete next[id];
			deps.setActiveMeetings(next);
			return;
		}
		deps.setActiveMeetings({ ...activeMeetings, [id]: info });
	}

	return {
		applyMeetingEvent,
		refreshGroupMeeting,
		setActiveMeeting
	};
}

export type MeetingsApi = ReturnType<typeof createMeetingsApi>;
