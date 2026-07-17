/**
 * Durable groups: create/join/leave, meta, members, announcements, roles.
 */

import { friendService, groupService } from '$lib/api';
import type {
	ChatMessage,
	GroupAnnouncement,
	GroupInfo,
	GroupMember
} from '../types';
import { appendUnique, saveJoinedGroups } from './joined-groups';
import { normalizeGroupMembers } from './normalize';

export interface GroupsDeps {
	getMyUserId: () => string;
	getGroupId: () => string;
	setGroupId: (v: string) => void;
	getTargetUser: () => string;
	getJoinedGroups: () => string[];
	setJoinedGroups: (g: string[]) => void;
	getGroupMeta: () => Record<string, GroupInfo>;
	setGroupMeta: (m: Record<string, GroupInfo>) => void;
	getGroupMembers: () => GroupMember[];
	setGroupMembers: (m: GroupMember[]) => void;
	/** Active conversation pins (group or private). */
	getGroupAnnouncements: () => GroupAnnouncement[];
	setGroupAnnouncements: (a: GroupAnnouncement[]) => void;
	getSelectMode: () => boolean;
	setSelectMode: (v: boolean) => void;
	getSelectedMsgIds: () => string[];
	setSelectedMsgIds: (ids: string[]) => void;
	getMessages: () => ChatMessage[];
	setMessages: (m: ChatMessage[]) => void;
	getChatMode: () => string;
	displayName: (uid: string) => string;
	wsSendJSON?: (payload: unknown) => Promise<void>;
	isWsOpen?: () => boolean;
}

export function createGroupsApi(deps: GroupsDeps) {
	async function refreshMyGroups() {
		try {
			const res = await groupService.listMine();
			const list = res.groups ?? [];
			const ids: string[] = [];
			const meta: Record<string, GroupInfo> = {};
			for (const g of list) {
				if (!g?.id) continue;
				ids.push(g.id);
				meta[g.id] = g;
			}
			deps.setJoinedGroups(ids);
			saveJoinedGroups(ids);
			deps.setGroupMeta({ ...deps.getGroupMeta(), ...meta });
		} catch {
			// ignore
		}
	}

	async function refreshGroupMembers(g?: string) {
		const gid = (g ?? deps.getGroupId()).trim();
		if (!gid) {
			deps.setGroupMembers([]);
			return;
		}
		try {
			const res = await groupService.members(gid);
			if (deps.getGroupId().trim() !== gid) return;
			deps.setGroupMembers(normalizeGroupMembers(res.members));
		} catch {
			// ignore
		}
	}

	/** Refresh pins for active conversation (group announcements or private pins). */
	async function refreshAnnouncements(scopeId?: string) {
		const mode = deps.getChatMode();
		if (mode === 'private') {
			const peer = (scopeId ?? deps.getTargetUser()).trim();
			if (!peer) {
				deps.setGroupAnnouncements([]);
				return;
			}
			try {
				const res = await friendService.listPins(peer);
				if (deps.getChatMode() !== 'private' || deps.getTargetUser().trim() !== peer) return;
				const pins = (res.pins ?? []).map((p) => ({
					id: p.id,
					peer_id: p.peer_id || peer,
					message_id: p.message_id,
					content: p.content,
					content_type: p.content_type,
					from_user_id: p.from_user_id,
					from_username: p.from_username,
					set_by_user_id: p.set_by_user_id,
					message_ts: p.message_ts,
					created_at: p.created_at
				})) satisfies GroupAnnouncement[];
				deps.setGroupAnnouncements(pins);
			} catch {
				// ignore (e.g. not friends)
			}
			return;
		}
		const gid = (scopeId ?? deps.getGroupId()).trim();
		if (!gid) {
			deps.setGroupAnnouncements([]);
			return;
		}
		try {
			const res = await groupService.listAnnouncements(gid);
			if (deps.getChatMode() !== 'group' || deps.getGroupId().trim() !== gid) return;
			deps.setGroupAnnouncements(res.announcements ?? []);
		} catch {
			// ignore
		}
	}

	function groupDisplayName(id: string): string {
		return deps.getGroupMeta()[id]?.name || id;
	}

	function isGroupOwner(id: string): boolean {
		const meta = deps.getGroupMeta()[id.trim()];
		if (!meta) return false;
		const r = String(meta.role ?? '').toLowerCase();
		if (r === 'owner') return true;
		if (r === 'admin' || r === 'member') return false;
		return String(meta.owner_user_id ?? '') === String(deps.getMyUserId());
	}

	function isGroupManager(gid: string): boolean {
		const id = gid.trim();
		if (!id) return false;
		if (isGroupOwner(id)) return true;
		return deps.getGroupMeta()[id]?.role === 'admin';
	}

	async function createGroup(name?: string, customId?: string) {
		const g = await groupService.create({
			name: name?.trim() || undefined,
			group_id: customId?.trim() || undefined
		});
		deps.setJoinedGroups(appendUnique(deps.getJoinedGroups(), g.id));
		saveJoinedGroups(deps.getJoinedGroups());
		deps.setGroupMeta({ ...deps.getGroupMeta(), [g.id]: g });
		return g;
	}

	async function uploadGroupAvatar(groupId: string, file: File) {
		const gid = groupId.trim();
		if (!gid) throw new Error('group_id required');
		const res = await groupService.uploadAvatar(gid, file);
		if (res.group) {
			deps.setGroupMeta({
				...deps.getGroupMeta(),
				[gid]: { ...deps.getGroupMeta()[gid], ...res.group }
			});
		} else {
			deps.setGroupMeta({
				...deps.getGroupMeta(),
				[gid]: {
					...(deps.getGroupMeta()[gid] || {
						id: gid,
						name: gid,
						owner_user_id: deps.getMyUserId()
					}),
					avatar: res.avatar,
					avatar_rev: res.avatar_rev
				}
			});
		}
		return res;
	}

	async function renameGroup(groupIdArg: string, name: string) {
		const gid = groupIdArg.trim();
		const n = name.trim();
		if (!gid) throw new Error('group_id required');
		if (!n) throw new Error('群名不能为空');
		const g = await groupService.update(gid, { name: n });
		deps.setGroupMeta({ ...deps.getGroupMeta(), [gid]: { ...deps.getGroupMeta()[gid], ...g } });
		return g;
	}

	async function setMemberRole(
		groupIdArg: string,
		userId: string,
		role: 'admin' | 'member'
	) {
		const gid = groupIdArg.trim();
		const uid = userId.trim();
		if (!gid || !uid) throw new Error('参数不完整');
		const m = await groupService.setMemberRole(gid, uid, role);
		if (deps.getGroupId() === gid && deps.getGroupMembers().length) {
			deps.setGroupMembers(
				normalizeGroupMembers(
					deps.getGroupMembers().map((x) =>
						x.user_id === uid ? { ...x, role: m.role || role } : x
					)
				)
			);
		}
		return m;
	}

	async function dissolveGroup(g: string) {
		const id = g.trim();
		if (!id) return;
		await groupService.dissolve(id);
		deps.setJoinedGroups(deps.getJoinedGroups().filter((g2) => g2 !== id));
		saveJoinedGroups(deps.getJoinedGroups());
		const nextMeta = { ...deps.getGroupMeta() };
		delete nextMeta[id];
		deps.setGroupMeta(nextMeta);
		if (deps.getChatMode() === 'group' && deps.getGroupId() === id) {
			deps.setMessages([]);
			deps.setGroupId('');
			deps.setGroupMembers([]);
			deps.setGroupAnnouncements([]);
		}
	}

	function enterSelectMode(seedId?: string) {
		const mode = deps.getChatMode();
		if (mode !== 'group' && mode !== 'private') return;
		if (mode === 'group' && !deps.getGroupId().trim()) return;
		if (mode === 'private' && !deps.getTargetUser().trim()) return;
		deps.setSelectMode(true);
		deps.setSelectedMsgIds(seedId ? [seedId] : []);
	}

	function exitSelectMode() {
		deps.setSelectMode(false);
		deps.setSelectedMsgIds([]);
	}

	function toggleSelectMessage(msgId: string) {
		if (!msgId) return;
		if (!deps.getSelectMode()) {
			enterSelectMode(msgId);
			return;
		}
		const cur = deps.getSelectedMsgIds();
		if (cur.includes(msgId)) {
			const next = cur.filter((id) => id !== msgId);
			deps.setSelectedMsgIds(next);
			if (next.length === 0) deps.setSelectMode(false);
		} else {
			deps.setSelectedMsgIds([...cur, msgId]);
		}
	}

	function buildPinItems(ids: string[]) {
		const byId = new Map(
			deps.getMessages().filter((m) => m.id).map((m) => [m.id as string, m])
		);
		return ids.map((id) => {
			const m = byId.get(id);
			const content =
				m?.content_type === 'voice'
					? '[语音]'
					: m?.content_type === 'red_packet'
						? '[红包]'
						: (m?.content || '').trim() || '[消息]';
			return {
				message_id: id,
				content: content.slice(0, 500),
				content_type: m?.content_type || 'text',
				from_user_id: m?.from,
				from_username: deps.displayName(m?.from || '') || m?.from,
				message_ts: m?.timestamp
			};
		});
	}

	function mergePins(added: GroupAnnouncement[]) {
		const map = new Map(deps.getGroupAnnouncements().map((a) => [a.message_id, a]));
		for (const a of added) map.set(a.message_id, a);
		deps.setGroupAnnouncements(
			Array.from(map.values()).sort((a, b) => (b.created_at || 0) - (a.created_at || 0))
		);
	}

	/** Pin messages in active group or private chat (append; multiple allowed). */
	async function setMessagesAsAnnouncement(msgIds?: string[]): Promise<GroupAnnouncement[]> {
		const mode = deps.getChatMode();
		const ids = (msgIds?.length ? msgIds : deps.getSelectedMsgIds()).filter(Boolean);
		if (!ids.length) throw new Error('请先选择消息');
		const items = buildPinItems(ids);

		if (mode === 'private') {
			const peer = deps.getTargetUser().trim();
			if (!peer) throw new Error('请先选择私聊');
			const res = await friendService.addPins(peer, { items });
			const added = (res.pins ?? []).map((p) => ({
				id: p.id,
				peer_id: p.peer_id || peer,
				message_id: p.message_id,
				content: p.content,
				content_type: p.content_type,
				from_user_id: p.from_user_id,
				from_username: p.from_username,
				set_by_user_id: p.set_by_user_id,
				message_ts: p.message_ts,
				created_at: p.created_at
			})) satisfies GroupAnnouncement[];
			mergePins(added);
			exitSelectMode();
			return added;
		}

		const gid = deps.getGroupId().trim();
		if (mode !== 'group' || !gid) throw new Error('请先选择会话');
		const res = await groupService.addAnnouncements(gid, { items });
		const added = res.announcements ?? [];
		mergePins(added);
		exitSelectMode();
		return added;
	}

	async function removeAnnouncement(messageId: string) {
		if (!messageId) return;
		const mode = deps.getChatMode();
		if (mode === 'private') {
			const peer = deps.getTargetUser().trim();
			if (!peer) return;
			await friendService.removePin(peer, messageId);
		} else {
			const gid = deps.getGroupId().trim();
			if (!gid) return;
			await groupService.removeAnnouncement(gid, messageId);
		}
		deps.setGroupAnnouncements(
			deps.getGroupAnnouncements().filter((a) => a.message_id !== messageId)
		);
	}

	function isAnnouncement(messageId: string | undefined): boolean {
		if (!messageId) return false;
		return deps.getGroupAnnouncements().some((a) => a.message_id === messageId);
	}

	return {
		refreshMyGroups,
		refreshGroupMembers,
		refreshAnnouncements,
		groupDisplayName,
		isGroupOwner,
		isGroupManager,
		createGroup,
		uploadGroupAvatar,
		renameGroup,
		setMemberRole,
		dissolveGroup,
		enterSelectMode,
		exitSelectMode,
		toggleSelectMessage,
		setMessagesAsAnnouncement,
		removeAnnouncement,
		isAnnouncement
	};
}

export type GroupsApi = ReturnType<typeof createGroupsApi>;
