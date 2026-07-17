/**
 * LiveKit session controller (Svelte 5 runes).
 *
 * Two distinct modes:
 *
 * Private 1:1 call (ring):
 *  1) Caller: token → signal invite → join room → ringback
 *  2) Callee: WS invite → ringtone → accept → token(same room) → join
 *  3) Hangup: signal end/cancel first, then leave room once
 *
 * Group conference (meeting mode — NOT a ring call):
 *  1) Host: POST meeting/start → join LiveKit → members see "会议进行中"
 *  2) Member: POST meeting/join → enter room (no accept/reject ring)
 *  3) Leave: only self leaves; meeting ends when last person leaves or host ends
 *
 * Fixes "Client initiated disconnect" / DUPLICATE_IDENTITY:
 *  - single in-flight connect (connectGen)
 *  - never disconnect twice
 *  - Disconnected handler does not call disconnect again
 *  - unique room name per private call session
 */

import {
	ConnectionState,
	Room,
	RoomEvent,
	Track,
	type RemoteParticipant,
	type RemoteTrack,
	type RemoteTrackPublication
} from 'livekit-client';
import {
	livekitService,
	type CallType,
	type LiveKitTokenResponse,
	type MeetingStatus
} from '$lib/api';
import type { CallEvent, CallMedia, MeetingEvent } from './types';
import {
	playConnectTone,
	playEndTone,
	startRingback,
	startRingtone,
	stopCallSounds
} from './call-sounds';

export type CallPhase =
	| 'idle'
	| 'outgoing'
	| 'incoming'
	| 'connecting'
	| 'connected'
	| 'ended';

function shortId(): string {
	return Math.random().toString(36).slice(2, 8);
}

/** Stable pair prefix + unique session so re-dials don't collide. */
function privateSessionRoom(a: string, b: string): string {
	const ids = [a, b].sort();
	return `dm_${ids[0]}_${ids[1]}_${shortId()}`;
}

export function createCallController(opts: {
	userId: string;
	username: string;
}) {
	let phase = $state<CallPhase>('idle');
	let callType = $state<CallType>('private');
	/** audio = 语音通话 (mic only); video = 视讯 (mic + camera) */
	let mediaMode = $state<CallMedia>('video');
	let roomName = $state('');
	let peerId = $state('');
	let peerName = $state('');
	let groupId = $state('');
	let error = $state('');
	let micEnabled = $state(true);
	let camEnabled = $state(false);
	let participants = $state<{ identity: string; name: string }[]>([]);

	let room: Room | null = null;
	let localVideoEl: HTMLVideoElement | null = null;
	const remoteVideos = new Map<string, HTMLVideoElement>();

	/** Bumps on each connect attempt; stale connects abort. */
	let connectGen = 0;
	/** True while we intentionally leave — ignore remote end races. */
	let leaving = false;
	/** Prevent overlapping start/accept. */
	let busy = false;
	/**
	 * Grace timer: LiveKit fires ParticipantDisconnected on brief reconnect /
	 * DUPLICATE_IDENTITY. Ending immediately makes "接听后马上挂断".
	 */
	let peerLeaveTimer: ReturnType<typeof setTimeout> | null = null;
	/** True once we have ever seen a remote participant this session. */
	let hadRemote = false;

	function clearPeerLeaveTimer() {
		if (peerLeaveTimer != null) {
			clearTimeout(peerLeaveTimer);
			peerLeaveTimer = null;
		}
	}

	function reset() {
		clearPeerLeaveTimer();
		phase = 'idle';
		callType = 'private';
		mediaMode = 'video';
		roomName = '';
		peerId = '';
		peerName = '';
		groupId = '';
		error = '';
		micEnabled = true;
		camEnabled = false;
		participants = [];
		leaving = false;
		busy = false;
		hadRemote = false;
	}

	function isVideoCall(): boolean {
		return mediaMode === 'video';
	}

	async function attachLocal() {
		if (!room || !localVideoEl) return;
		const pub = room.localParticipant.getTrackPublication(Track.Source.Camera);
		const track = pub?.track;
		if (track) {
			track.attach(localVideoEl);
		}
	}

	function attachRemote(identity: string, track: RemoteTrack) {
		const el = remoteVideos.get(identity);
		if (el && track.kind === Track.Kind.Video) {
			track.attach(el);
		}
		if (track.kind === Track.Kind.Audio) {
			// Avoid duplicate audio elements for same identity+track.
			const sel = `audio[data-lk-identity="${identity}"][data-lk-sid="${track.sid}"]`;
			if (document.querySelector(sel)) return;
			const audio = track.attach() as HTMLAudioElement;
			audio.autoplay = true;
			audio.dataset.lkIdentity = identity;
			audio.dataset.lkSid = track.sid;
			document.body.appendChild(audio);
		}
	}

	function detachRemote(identity: string) {
		const el = remoteVideos.get(identity);
		if (el) el.srcObject = null;
		document.querySelectorAll(`audio[data-lk-identity="${identity}"]`).forEach((n) => n.remove());
	}

	function refreshParticipants() {
		if (!room) {
			participants = [];
			return;
		}
		const list: { identity: string; name: string }[] = [];
		room.remoteParticipants.forEach((p) => {
			list.push({ identity: p.identity, name: p.name || p.identity });
		});
		participants = list;
	}

	function bindRoom(r: Room, gen: number) {
		r.on(
			RoomEvent.TrackSubscribed,
			(track: RemoteTrack, _pub: RemoteTrackPublication, participant: RemoteParticipant) => {
				if (gen !== connectGen) return;
				attachRemote(participant.identity, track);
				refreshParticipants();
			}
		);
		r.on(RoomEvent.TrackUnsubscribed, (track: RemoteTrack, _pub, participant) => {
			track.detach();
			if (participant) detachRemote(participant.identity);
			refreshParticipants();
		});
		r.on(RoomEvent.ParticipantConnected, (p) => {
			if (gen !== connectGen) return;
			console.info('[call] remote joined', p.identity);
			// Peer re-appeared (or first join) — cancel any pending hangup.
			clearPeerLeaveTimer();
			hadRemote = true;
			refreshParticipants();
			// First remote join ends ringback / waiting UI.
			if (phase === 'outgoing' || phase === 'connecting') {
				stopCallSounds();
				void playConnectTone();
				phase = 'connected';
			}
		});
		r.on(RoomEvent.ParticipantDisconnected, (p) => {
			if (gen !== connectGen) return;
			console.info('[call] remote left', p.identity);
			detachRemote(p.identity);
			refreshParticipants();

			// Group: stay in room even if one person leaves.
			if (callType !== 'private' || leaving) return;
			// Never hang up instantly — LiveKit may briefly remove a participant
			// during DUPLICATE_IDENTITY / ICE reconnect (looks like "peer left").
			clearPeerLeaveTimer();
			// Only schedule hangup if we were actually talking (had a remote before).
			if (!hadRemote || phase === 'outgoing' || phase === 'connecting') {
				return;
			}
			const leaveGen = connectGen;
			peerLeaveTimer = setTimeout(() => {
				peerLeaveTimer = null;
				if (leaveGen !== connectGen || leaving) return;
				if (phase !== 'connected') return;
				// Still empty after grace → peer really hung up.
				if (room && room.remoteParticipants.size === 0) {
					console.info('[call] peer leave grace expired — ending call');
					void endLocal('peer_left');
				}
			}, 5000);
		});
		r.on(RoomEvent.Disconnected, (reason) => {
			// Only handle if this is still the active room generation.
			if (gen !== connectGen) return;
			console.info('[call] Room disconnected', reason);
			// Do NOT call room.disconnect() again — that causes "Client initiated disconnect" loops.
			if (room === r) {
				room = null;
			}
			stopCallSounds();
			clearPeerLeaveTimer();
			if (!leaving && phase !== 'idle' && phase !== 'ended') {
				// Unexpected disconnect (network / server). Don't thrash reconnect.
				phase = 'ended';
				error = reason ? `连接断开: ${String(reason)}` : '连接已断开';
				setTimeout(() => {
					if (phase === 'ended') reset();
				}, 1200);
			}
		});
		r.on(RoomEvent.LocalTrackPublished, () => {
			if (gen !== connectGen) return;
			void attachLocal();
		});
		r.on(RoomEvent.MediaDevicesError, (e) => {
			console.warn('[call] media device error', e);
			error = (e as Error)?.message || 'Camera/microphone error';
		});
	}

	/**
	 * Tear down media. Safe to call multiple times.
	 * stopDisconnect: if true, skip LiveKit leave (room already closed).
	 */
	function cleanupMedia(optsClean: { disconnect?: boolean } = { disconnect: true }) {
		const r = room;
		room = null;
		if (r) {
			r.remoteParticipants.forEach((p) => detachRemote(p.identity));
			try {
				r.localParticipant.trackPublications.forEach((pub) => {
					pub.track?.detach();
				});
			} catch {
				// ignore
			}
			if (optsClean.disconnect !== false && r.state !== ConnectionState.Disconnected) {
				try {
					// stopTracks=true releases camera/mic
					void r.disconnect(true);
				} catch {
					// ignore
				}
			}
		}
		if (localVideoEl) localVideoEl.srcObject = null;
		document.querySelectorAll('audio[data-lk-identity]').forEach((n) => n.remove());
	}

	async function connectWithToken(tok: LiveKitTokenResponse, gen: number) {
		if (gen !== connectGen) return;

		// Drop any previous room before opening a new one (prevents DUPLICATE_IDENTITY).
		if (room) {
			cleanupMedia({ disconnect: true });
		}

		phase = 'connecting';
		error = '';
		roomName = tok.room;
		callType = tok.call_type === 'group' ? 'group' : 'private';
		if (tok.peer_id) peerId = tok.peer_id;
		if (tok.group_id) groupId = tok.group_id;

		const r = new Room({
			adaptiveStream: true,
			dynacast: true,
			disconnectOnPageLeave: true
		});
		bindRoom(r, gen);
		room = r;

		// URL comes from backend only (LIVEKIT_URL=auto → same origin, nginx /rtc → livekit).
		const lkUrl = tok.url;
		try {
			console.info('[call] connecting', { url: lkUrl, room: tok.room, media: mediaMode });
			await r.connect(lkUrl, tok.token, { autoSubscribe: true });
			if (gen !== connectGen) {
				// Stale connect — leave immediately.
				try {
					void r.disconnect(true);
				} catch {
					// ignore
				}
				return;
			}

			// Mic always for both modes; camera only for video calls.
			try {
				await r.localParticipant.setMicrophoneEnabled(true);
				micEnabled = true;
			} catch (e) {
				console.warn('[call] mic failed', e);
				micEnabled = false;
				error = '无法打开麦克风';
			}
			if (isVideoCall()) {
				try {
					await r.localParticipant.setCameraEnabled(true);
					camEnabled = true;
				} catch (e) {
					console.warn('[call] camera failed', e);
					camEnabled = false;
					// Fall back to audio-only if camera denied mid video call.
					if (!error) error = '无法打开摄像头，已用语音模式';
				}
			} else {
				// Voice call: never touch camera (setCameraEnabled(false) can spuriously fail).
				camEnabled = false;
			}

			if (gen !== connectGen) return;
			if (isVideoCall()) await attachLocal();
			// Already-present remotes (caller waiting when callee joins).
			if (r.remoteParticipants.size > 0) {
				hadRemote = true;
				clearPeerLeaveTimer();
			}
			refreshParticipants();

			// Stay "outgoing" until peer joins (ringback continues); otherwise connected.
			if (phase === 'connecting') {
				if (r.remoteParticipants.size > 0) {
					stopCallSounds();
					void playConnectTone();
					phase = 'connected';
				} else if (callType === 'group') {
					// Group meeting: alone is fine.
					stopCallSounds();
					phase = 'connected';
				} else {
					// Private: waiting for peer — keep outgoing + ringback.
					phase = 'outgoing';
				}
			}
			console.info('[call] connected ok', {
				phase,
				remotes: r.remoteParticipants.size,
				media: mediaMode
			});
		} catch (e) {
			if (gen !== connectGen) return;
			const msg = (e as Error).message || 'Failed to connect';
			console.error('[call] connect failed', e);
			error = msg;
			cleanupMedia({ disconnect: true });
			stopCallSounds();
			phase = 'idle';
			throw e;
		}
	}

	/** Start private call: unique room → invite → join → ringback.
	 *  media: 'audio' = 语音, 'video' = 视讯
	 */
	async function startPrivateCall(
		toUserId: string,
		toName?: string,
		media: CallMedia = 'audio'
	) {
		if (busy || phase !== 'idle') throw new Error('Already in a call');
		busy = true;
		leaving = false;
		peerId = toUserId;
		peerName = toName || toUserId;
		callType = 'private';
		mediaMode = media === 'video' ? 'video' : 'audio';
		phase = 'outgoing';
		error = '';
		const gen = ++connectGen;
		const sessionRoom = privateSessionRoom(opts.userId, toUserId);
		roomName = sessionRoom;

		try {
			void startRingback();
			const tok = await livekitService.createToken({
				type: 'private',
				peer_id: toUserId,
				room: sessionRoom
			});
			if (gen !== connectGen) return;
			roomName = tok.room;
			await livekitService.signal({
				action: 'invite',
				room: tok.room,
				call_type: 'private',
				media: mediaMode,
				to: toUserId,
				from_name: opts.username
			});
			if (gen !== connectGen) return;
			await connectWithToken(tok, gen);
		} catch (e) {
			if (gen === connectGen) {
				error = (e as Error).message || 'Call failed';
				stopCallSounds();
				cleanupMedia();
				reset();
			}
			throw e;
		} finally {
			if (gen === connectGen) busy = false;
		}
	}

	function meetingStatusToToken(st: MeetingStatus): LiveKitTokenResponse {
		return {
			token: st.token || '',
			url: st.url || '',
			room: st.room || '',
			identity: st.identity || opts.userId,
			call_type: 'group',
			group_id: st.group_id,
			media: st.media
		};
	}

	/**
	 * Open a group conference (meeting mode).
	 * Does NOT ring members — they see "会议进行中" and join freely.
	 * If a meeting is already open, this joins it.
	 */
	async function startGroupMeeting(gid: string, media: CallMedia = 'audio') {
		if (busy || phase !== 'idle') throw new Error('Already in a call');
		busy = true;
		leaving = false;
		groupId = gid;
		callType = 'group';
		mediaMode = media === 'video' ? 'video' : 'audio';
		phase = 'connecting';
		error = '';
		const gen = ++connectGen;

		try {
			const st = await livekitService.meeting({
				group_id: gid,
				action: 'start',
				media: mediaMode
			});
			if (gen !== connectGen) return;
			if (!st.token || !st.url || !st.room) {
				throw new Error('会议 token 无效');
			}
			// Existing meeting may be audio while we requested video — follow server media.
			mediaMode = st.media === 'video' ? 'video' : 'audio';
			roomName = st.room;
			const tok = meetingStatusToToken(st);
			await connectWithToken(tok, gen);
			if (gen === connectGen) {
				void playConnectTone();
				phase = 'connected';
			}
		} catch (e) {
			if (gen === connectGen) {
				error = (e as Error).message || '开启会议失败';
				stopCallSounds();
				cleanupMedia();
				reset();
			}
			throw e;
		} finally {
			if (gen === connectGen) busy = false;
		}
	}

	/** Join an already-open group meeting (no ring / accept). */
	async function joinGroupMeeting(gid: string) {
		if (busy || phase !== 'idle') throw new Error('Already in a call');
		busy = true;
		leaving = false;
		groupId = gid;
		callType = 'group';
		phase = 'connecting';
		error = '';
		const gen = ++connectGen;

		try {
			const st = await livekitService.meeting({ group_id: gid, action: 'join' });
			if (gen !== connectGen) return;
			if (!st.token || !st.url || !st.room) {
				throw new Error('加入会议失败：无 token');
			}
			mediaMode = st.media === 'video' ? 'video' : 'audio';
			roomName = st.room;
			await connectWithToken(meetingStatusToToken(st), gen);
			if (gen === connectGen) {
				void playConnectTone();
				phase = 'connected';
			}
		} catch (e) {
			if (gen === connectGen) {
				error = (e as Error).message || '加入会议失败';
				stopCallSounds();
				cleanupMedia();
				reset();
			}
			throw e;
		} finally {
			if (gen === connectGen) busy = false;
		}
	}

	/** Private ring only — group never uses ringtone invite. */
	function onIncoming(ev: CallEvent) {
		// Legacy group invite signals are ignored (meeting mode uses MeetingEvent).
		if (ev.call_type === 'group') return;

		if (phase !== 'idle') {
			if (ev.from) {
				void livekitService
					.signal({
						action: 'reject',
						room: ev.room,
						call_type: 'private',
						media: (ev.media as CallMedia) || 'audio',
						to: ev.from
					})
					.catch(() => undefined);
			}
			return;
		}
		phase = 'incoming';
		callType = 'private';
		mediaMode = ev.media === 'video' ? 'video' : 'audio';
		roomName = ev.room;
		peerId = ev.from;
		peerName = ev.from_name || ev.from;
		groupId = '';
		error = '';
		void startRingtone();
	}

	async function acceptIncoming() {
		if (phase !== 'incoming' || !roomName || busy || callType !== 'private') return;
		busy = true;
		const gen = ++connectGen;
		stopCallSounds();
		try {
			const tok = await livekitService.createToken({
				type: 'private',
				peer_id: peerId,
				room: roomName
			});
			if (gen !== connectGen) return;
			if (peerId) {
				await livekitService.signal({
					action: 'accept',
					room: roomName,
					call_type: 'private',
					media: mediaMode,
					to: peerId,
					from_name: opts.username
				});
			}
			if (gen !== connectGen) return;
			await connectWithToken(tok, gen);
			if (gen === connectGen) {
				void playConnectTone();
				phase = 'connected';
			}
		} catch (e) {
			if (gen === connectGen) {
				error = (e as Error).message || 'Accept failed';
				stopCallSounds();
				cleanupMedia();
				reset();
			}
		} finally {
			if (gen === connectGen) busy = false;
		}
	}

	async function rejectIncoming() {
		if (phase !== 'incoming') return;
		stopCallSounds();
		const r = roomName;
		const p = peerId;
		reset();
		try {
			if (p) {
				await livekitService.signal({
					action: 'reject',
					room: r,
					call_type: 'private',
					to: p
				});
			}
		} catch {
			// ignore
		}
	}

	/** Local teardown after peer left or remote end signal. */
	async function endLocal(reason: string) {
		if (leaving || phase === 'idle' || phase === 'ended') return;
		console.info('[call] endLocal', reason);
		leaving = true;
		clearPeerLeaveTimer();
		connectGen++; // invalidate in-flight connects
		stopCallSounds();
		void playEndTone();
		cleanupMedia({ disconnect: true });
		phase = 'ended';
		if (reason === 'peer_left') {
			error = '对方已挂断';
		} else if (reason === 'reject') {
			error = '对方已拒绝';
		} else if (reason === 'cancel') {
			error = '对方已取消';
		}
		setTimeout(() => {
			if (phase === 'ended') reset();
		}, 1200);
	}

	/**
	 * Hang up / leave.
	 * - Private: cancel/end signals the peer (call ends for both).
	 * - Group meeting: only leave yourself (others stay in the conference).
	 */
	async function hangup() {
		if (phase === 'idle' || leaving) return;
		leaving = true;
		const prevPhase = phase;
		const prevType = callType;
		const prevRoom = roomName;
		const prevPeer = peerId;
		const prevGroup = groupId;

		connectGen++; // cancel any connecting attempt
		stopCallSounds();
		void playEndTone();

		// Signal BEFORE tearing down media.
		try {
			if (prevType === 'group' && prevGroup) {
				// Meeting leave — does not kick others.
				await livekitService.meeting({ group_id: prevGroup, action: 'leave' });
			} else if (prevPhase === 'incoming') {
				if (prevPeer) {
					await livekitService.signal({
						action: 'reject',
						room: prevRoom,
						call_type: 'private',
						to: prevPeer
					});
				}
			} else if (
				prevPhase === 'outgoing' ||
				prevPhase === 'connected' ||
				prevPhase === 'connecting'
			) {
				if (prevPeer) {
					await livekitService.signal({
						action:
							prevPhase === 'outgoing' || prevPhase === 'connecting' ? 'cancel' : 'end',
						room: prevRoom,
						call_type: 'private',
						to: prevPeer
					});
				}
			}
		} catch {
			// ignore signal errors
		}

		cleanupMedia({ disconnect: true });
		reset();
	}

	/** Force-end group meeting for everyone (host / any member). */
	async function endGroupMeeting() {
		if (callType !== 'group' || !groupId || leaving) return;
		leaving = true;
		const gid = groupId;
		connectGen++;
		stopCallSounds();
		void playEndTone();
		try {
			await livekitService.meeting({ group_id: gid, action: 'end' });
		} catch {
			// ignore
		}
		cleanupMedia({ disconnect: true });
		reset();
	}

	/** Handle remote private call signaling from chat WS. */
	function handleCallEvent(ev: CallEvent) {
		if (!ev || ev.type !== 'call') return;
		if (ev.from === opts.userId) return;
		// Group ring invites are obsolete — meeting mode only.
		if (ev.call_type === 'group') return;

		switch (ev.action) {
			case 'invite':
				onIncoming(ev);
				break;
			case 'accept':
				console.info('[call] peer accepted', ev.room);
				break;
			case 'reject':
			case 'cancel':
			case 'end':
				if (roomName && ev.room && roomName !== ev.room) return;
				if (phase === 'idle' || phase === 'ended') return;
				if (callType !== 'private') return;
				void endLocal(ev.action);
				break;
			default:
				break;
		}
	}

	/**
	 * Group meeting lifecycle from WS.
	 * When meeting is ended by someone else, leave media if we are in that room.
	 */
	function handleMeetingEvent(ev: MeetingEvent) {
		if (!ev || ev.type !== 'meeting') return;
		if (ev.from === opts.userId) return;
		if (ev.action !== 'ended') return;
		if (callType !== 'group') return;
		if (groupId && ev.group_id && groupId !== ev.group_id) return;
		if (phase === 'idle' || phase === 'ended') return;
		void endLocal('end');
	}

	function setLocalVideoEl(el: HTMLVideoElement | null) {
		localVideoEl = el;
		void attachLocal();
	}

	function setRemoteVideoEl(identity: string, el: HTMLVideoElement | null) {
		if (!el) {
			remoteVideos.delete(identity);
			return;
		}
		remoteVideos.set(identity, el);
		if (room) {
			const p = room.remoteParticipants.get(identity);
			const pub = p?.getTrackPublication(Track.Source.Camera);
			const track = pub?.track;
			if (track) track.attach(el);
		}
	}

	async function toggleMic() {
		if (!room) return;
		const next = !micEnabled;
		try {
			await room.localParticipant.setMicrophoneEnabled(next);
			micEnabled = next;
		} catch (e) {
			console.warn('[call] toggle mic', e);
		}
	}

	async function toggleCam() {
		if (!room) return;
		// Voice-only calls cannot enable camera mid-call (switch would need re-invite).
		if (!isVideoCall()) {
			error = '语音通话无法开启摄像头，请改用视讯通话';
			return;
		}
		const next = !camEnabled;
		try {
			await room.localParticipant.setCameraEnabled(next);
			camEnabled = next;
			if (next) await attachLocal();
		} catch (e) {
			console.warn('[call] toggle cam', e);
		}
	}

	function dispose() {
		leaving = true;
		connectGen++;
		stopCallSounds();
		cleanupMedia({ disconnect: true });
		reset();
	}

	return {
		get phase() {
			return phase;
		},
		get callType() {
			return callType;
		},
		get mediaMode() {
			return mediaMode;
		},
		get isVideo() {
			return mediaMode === 'video';
		},
		get roomName() {
			return roomName;
		},
		get peerId() {
			return peerId;
		},
		get peerName() {
			return peerName;
		},
		get groupId() {
			return groupId;
		},
		get error() {
			return error;
		},
		get micEnabled() {
			return micEnabled;
		},
		get camEnabled() {
			return camEnabled;
		},
		get participants() {
			return participants;
		},
		get connectionState() {
			return room?.state ?? ConnectionState.Disconnected;
		},
		startPrivateCall,
		startGroupMeeting,
		joinGroupMeeting,
		acceptIncoming,
		rejectIncoming,
		hangup,
		endGroupMeeting,
		handleCallEvent,
		handleMeetingEvent,
		setLocalVideoEl,
		setRemoteVideoEl,
		toggleMic,
		toggleCam,
		dispose
	};
}

export type CallController = ReturnType<typeof createCallController>;
