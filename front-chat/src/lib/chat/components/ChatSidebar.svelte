<script lang="ts">
	import type {
		ActiveGroupMeeting,
		BlacklistUser,
		ChatMode,
		FriendRequest,
		FriendUser,
		GroupInfo,
		OnlineUser
	} from '../types';
	import { formatRelativeTime } from '../utils';
	import { groupAvatarUrl, groupService } from '$lib/api/group.service';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Badge } from '$lib/components/ui/badge';
	import { Separator } from '$lib/components/ui/separator';
	import * as Tabs from '$lib/components/ui/tabs';
	import * as ScrollArea from '$lib/components/ui/scroll-area';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import UserAvatar from './UserAvatar.svelte';
	import Users from '@lucide/svelte/icons/users';
	import Hash from '@lucide/svelte/icons/hash';
	import RefreshCw from '@lucide/svelte/icons/refresh-cw';
	import UserPlus from '@lucide/svelte/icons/user-plus';
	import LogOut from '@lucide/svelte/icons/log-out';
	import Check from '@lucide/svelte/icons/check';
	import X from '@lucide/svelte/icons/x';
	import UserMinus from '@lucide/svelte/icons/user-minus';
	import Plus from '@lucide/svelte/icons/plus';
	import Trash2 from '@lucide/svelte/icons/trash-2';
	import Crown from '@lucide/svelte/icons/crown';
	import Ban from '@lucide/svelte/icons/ban';
	import ShieldOff from '@lucide/svelte/icons/shield-off';
	import Phone from '@lucide/svelte/icons/phone';
	import Video from '@lucide/svelte/icons/video';
	import LoaderCircle from '@lucide/svelte/icons/loader-circle';
	import Search from '@lucide/svelte/icons/search';
	import MoreHorizontal from '@lucide/svelte/icons/more-horizontal';
	import ChevronDown from '@lucide/svelte/icons/chevron-down';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import Settings from '@lucide/svelte/icons/settings';
	import { confirmDialog } from '$lib/ui/notify.svelte';

	interface Props {
		chatMode: ChatMode;
		targetUser?: string;
		groupId?: string;
		joinedGroups: string[];
		groupMeta?: Record<string, GroupInfo>;
		/** Accepted friends — primary private list. */
		friends: FriendUser[];
		/** Pending invites I received. */
		incomingRequests: FriendRequest[];
		/** Users I blocked. */
		blacklist?: BlacklistUser[];
		/** Global online users (optional browse). */
		onlineUsers: OnlineUser[];
		myUserId: string;
		unreadPeers?: Record<string, boolean>;
		unreadGroups?: Record<string, boolean>;
		lastPreviews?: Record<string, { text: string; ts: number }>;
		/** Active group meetings keyed by group_id (list badges). */
		activeMeetings?: Record<string, ActiveGroupMeeting>;
		onModeChange: (mode: ChatMode) => void;
		onJoinGroup: () => void;
		onLeaveGroup: (g: string) => void;
		onCreateGroup: (name: string, customId?: string) => Promise<void>;
		onDissolveGroup: (g: string) => Promise<void>;
		onSelectGroup: (g: string) => void;
		onSelectUser: (userId: string, username?: string) => void;
		onRefreshOnline: () => void;
		onRefreshFriends: () => void;
		onRefreshGroups?: () => void;
		/** Open group settings (name / avatar / roles / dissolve). */
		onOpenGroupSettings?: (groupId: string) => void;
		onInviteFriend: (username: string) => Promise<void>;
		onAcceptRequest: (id: number) => Promise<void>;
		onRejectRequest: (id: number) => Promise<void>;
		onRemoveFriend: (userId: string) => Promise<void>;
		onBlockUser: (opts: { user_id?: string; username?: string }) => Promise<void>;
		onUnblockUser: (userId: string) => Promise<void>;
		/** Start a private LiveKit call with this friend. media: audio | video */
		onCallUser?: (
			userId: string,
			username?: string,
			media?: 'audio' | 'video'
		) => void | Promise<void>;
		callDisabled?: boolean;
	}

	let {
		chatMode,
		targetUser = $bindable(''),
		groupId = $bindable(''),
		joinedGroups,
		groupMeta = {},
		friends,
		incomingRequests,
		blacklist = [],
		onlineUsers,
		myUserId,
		unreadPeers = {},
		unreadGroups = {},
		lastPreviews = {},
		activeMeetings = {},
		onModeChange,
		onJoinGroup,
		onLeaveGroup,
		onCreateGroup,
		onDissolveGroup,
		onSelectGroup,
		onSelectUser,
		onRefreshOnline,
		onRefreshFriends,
		onRefreshGroups,
		onOpenGroupSettings,
		onInviteFriend,
		onAcceptRequest,
		onRejectRequest,
		onRemoveFriend,
		onBlockUser,
		onUnblockUser,
		onCallUser,
		callDisabled = false
	}: Props = $props();

	let joinGroupInput = $state('');
	let joinSuggestions = $state<GroupInfo[]>([]);
	let joinSearchBusy = $state(false);
	let joinSuggestOpen = $state(false);
	let joinSearchEmpty = $state(false);
	let joinHighlight = $state(-1);
	let joinSearchTimer: ReturnType<typeof setTimeout> | null = null;
	let joinSearchSeq = 0;
	let createName = $state('');
	let createId = $state('');
	let createBusy = $state(false);
	let createError = $state('');
	let inviteUsername = $state('');
	let inviteBusy = $state(false);
	let inviteError = $state('');
	let inviteOk = $state('');
	/** Private sidebar: search / sections */
	let friendFilter = $state('');
	let inviteOpen = $state(false);
	let onlineOpen = $state(false);
	let blacklistOpen = $state(false);
	/** Group sidebar: search / sections */
	let groupFilter = $state('');
	let createGroupOpen = $state(false);
	let joinGroupOpen = $state(true);

	const friendIds = $derived(new Set(friends.map((f) => f.user_id)));
	const blockedIds = $derived(new Set(blacklist.map((u) => u.user_id)));
	const othersOnline = $derived(
		onlineUsers.filter(
			(u) => u.user_id !== myUserId && !friendIds.has(u.user_id) && !blockedIds.has(u.user_id)
		)
	);

	/** Friends sorted: unread → online → last activity → name */
	const sortedFriends = $derived.by(() => {
		const q = friendFilter.trim().toLowerCase();
		const list = friends.filter((u) => {
			if (!q) return true;
			const name = (u.username || '').toLowerCase();
			const id = (u.user_id || '').toLowerCase();
			return name.includes(q) || id.includes(q);
		});
		return [...list].sort((a, b) => {
			const ua = !!unreadPeers[a.user_id];
			const ub = !!unreadPeers[b.user_id];
			if (ua !== ub) return ua ? -1 : 1;
			if (a.online !== b.online) return a.online ? -1 : 1;
			const ta = lastPreviews[`private:${a.user_id}`]?.ts ?? 0;
			const tb = lastPreviews[`private:${b.user_id}`]?.ts ?? 0;
			if (ta !== tb) return tb - ta;
			return (a.username || a.user_id).localeCompare(b.username || b.user_id, 'zh');
		});
	});

	const onlineFriendCount = $derived(friends.filter((f) => f.online).length);

	/** Groups sorted: unread → meeting → last activity → name */
	const sortedGroups = $derived.by(() => {
		const q = groupFilter.trim().toLowerCase();
		const list = joinedGroups.filter((id) => {
			if (!q) return true;
			const name = (groupLabel(id) || '').toLowerCase();
			const gid = id.toLowerCase();
			return name.includes(q) || gid.includes(q);
		});
		return [...list].sort((a, b) => {
			const ua = !!unreadGroups[a];
			const ub = !!unreadGroups[b];
			if (ua !== ub) return ua ? -1 : 1;
			const ma = !!activeMeetings[a];
			const mb = !!activeMeetings[b];
			if (ma !== mb) return ma ? -1 : 1;
			const ta = lastPreviews[`group:${a}`]?.ts ?? 0;
			const tb = lastPreviews[`group:${b}`]?.ts ?? 0;
			if (ta !== tb) return tb - ta;
			return groupLabel(a).localeCompare(groupLabel(b), 'zh');
		});
	});

	const ownerGroupCount = $derived(joinedGroups.filter((g) => isOwner(g)).length);

	function avatarUrl(uid: string): string {
		if (!uid) return '';
		return `/api/avatar/${encodeURIComponent(uid)}`;
	}

	/** Stable hue for group avatar tile. */
	function groupHue(id: string): number {
		let h = 0;
		for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) >>> 0;
		return (h % 300) + 20;
	}

	function groupInitial(id: string): string {
		const name = groupLabel(id).trim();
		return name ? name.slice(0, 1).toUpperCase() : '#';
	}

	function scheduleJoinSearch(q: string) {
		if (joinSearchTimer) clearTimeout(joinSearchTimer);
		const query = q.trim();
		if (query.length < 1) {
			joinSuggestions = [];
			joinSuggestOpen = false;
			joinSearchEmpty = false;
			joinHighlight = -1;
			return;
		}
		joinSearchTimer = setTimeout(() => {
			void runJoinSearch(query);
		}, 220);
	}

	async function runJoinSearch(q: string) {
		const seq = ++joinSearchSeq;
		joinSearchBusy = true;
		joinSearchEmpty = false;
		try {
			const res = await groupService.search(q, 12);
			if (seq !== joinSearchSeq) return;
			joinSuggestions = res.groups ?? [];
			joinSuggestOpen = joinSuggestions.length > 0;
			joinSearchEmpty = joinSuggestions.length === 0;
			joinHighlight = joinSuggestions.length > 0 ? 0 : -1;
		} catch {
			if (seq !== joinSearchSeq) return;
			joinSuggestions = [];
			joinSuggestOpen = false;
			joinSearchEmpty = true;
			joinHighlight = -1;
		} finally {
			if (seq === joinSearchSeq) joinSearchBusy = false;
		}
	}

	function pickJoinSuggestion(g: GroupInfo) {
		joinGroupInput = g.id;
		joinSuggestOpen = false;
		joinSuggestions = [];
		joinHighlight = -1;
		groupId = g.id;
		onJoinGroup();
	}

	function handleJoin() {
		const g = joinGroupInput.trim();
		if (!g) return;
		// If input matches a suggestion name-only, prefer highlighted / exact id match.
		const exact = joinSuggestions.find(
			(s) => s.id === g || s.name === g || s.id.toLowerCase() === g.toLowerCase()
		);
		const pick =
			exact ||
			(joinHighlight >= 0 && joinHighlight < joinSuggestions.length
				? joinSuggestions[joinHighlight]
				: null);
		const id = pick?.id || g;
		joinSuggestOpen = false;
		groupId = id;
		joinGroupInput = id;
		onJoinGroup();
	}

	function onJoinKeydown(e: KeyboardEvent) {
		if (!joinSuggestOpen || joinSuggestions.length === 0) {
			if (e.key === 'Enter') {
				e.preventDefault();
				handleJoin();
			}
			return;
		}
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			joinHighlight = (joinHighlight + 1) % joinSuggestions.length;
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			joinHighlight =
				joinHighlight <= 0 ? joinSuggestions.length - 1 : joinHighlight - 1;
		} else if (e.key === 'Enter') {
			e.preventDefault();
			const s = joinSuggestions[joinHighlight] ?? joinSuggestions[0];
			if (s) pickJoinSuggestion(s);
			else handleJoin();
		} else if (e.key === 'Escape') {
			joinSuggestOpen = false;
			joinHighlight = -1;
		}
	}

	const GROUP_ID_RE = /^[a-zA-Z0-9][a-zA-Z0-9_-]{2,63}$/;
	const createIdHint = $derived.by(() => {
		const id = createId.trim();
		if (!id) return '';
		if (id.length < 3 || id.length > 64) return '自定义 ID：3–64 个字符';
		if (!GROUP_ID_RE.test(id)) return '仅字母/数字/下划线/连字符，且以字母或数字开头';
		const reserved = ['search', 'join', 'leave', 'members', 'new', 'create', 'me', 'admin'];
		if (reserved.includes(id.toLowerCase())) return '该 ID 为系统保留字';
		return '';
	});
	const canCreate = $derived(
		createName.trim().length >= 2 &&
			createName.trim().length <= 40 &&
			!createIdHint &&
			!createBusy
	);

	async function handleCreate() {
		createError = '';
		const name = createName.trim();
		const id = createId.trim();
		if (name.length < 2) {
			createError = '请填写群名称（至少 2 个字）';
			return;
		}
		if (name.length > 40) {
			createError = '群名称最多 40 个字';
			return;
		}
		if (id && createIdHint) {
			createError = createIdHint;
			return;
		}
		createBusy = true;
		try {
			await onCreateGroup(name, id || undefined);
			createName = '';
			createId = '';
			createError = '';
			createGroupOpen = false;
		} catch (err) {
			createError = (err as Error).message || '创建失败';
		} finally {
			createBusy = false;
		}
	}

	function groupLabel(id: string): string {
		return groupMeta[id]?.name || id;
	}

	function isOwner(id: string): boolean {
		const m = groupMeta[id];
		return m?.role === 'owner' || m?.owner_user_id === myUserId;
	}

	async function handleInvite() {
		inviteError = '';
		inviteOk = '';
		const name = inviteUsername.trim();
		if (!name) return;
		inviteBusy = true;
		try {
			await onInviteFriend(name);
			inviteOk = `已向 ${name} 发送好友邀请，对方同意后才会成为好友`;
			inviteUsername = '';
		} catch (err) {
			inviteError = (err as Error).message || 'Invite failed';
		} finally {
			inviteBusy = false;
		}
	}
</script>

<aside class="bg-sidebar text-sidebar-foreground flex w-80 shrink-0 flex-col border-r">
	<div class="space-y-3 p-4">
		<Tabs.Root
			value={chatMode}
			onValueChange={(v) => {
				if (v === 'private' || v === 'group') onModeChange(v);
			}}
			class="w-full"
		>
			<Tabs.List class="grid w-full grid-cols-2">
				<Tabs.Trigger value="private" class="gap-1.5">
					<Users class="size-3.5" />
					私聊
				</Tabs.Trigger>
				<Tabs.Trigger value="group" class="gap-1.5">
					<Hash class="size-3.5" />
					群组
				</Tabs.Trigger>
			</Tabs.List>
		</Tabs.Root>

		{#if chatMode === 'private'}
			<!-- Search friends -->
			<div class="relative">
				<Search
					class="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2"
				/>
				<Input
					bind:value={friendFilter}
					placeholder="搜索好友…"
					class="h-9 pl-8"
					autocomplete="off"
				/>
			</div>
			<!-- Add friend (collapsible) -->
			<div class="rounded-lg border border-border/60 bg-muted/15">
				<button
					type="button"
					class="hover:bg-muted/40 flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition-colors"
					onclick={() => (inviteOpen = !inviteOpen)}
				>
					<div class="bg-primary/10 text-primary flex size-7 items-center justify-center rounded-md">
						<UserPlus class="size-3.5" />
					</div>
					<span class="min-w-0 flex-1 font-medium">添加好友</span>
					{#if inviteOpen}
						<ChevronDown class="text-muted-foreground size-4" />
					{:else}
						<ChevronRight class="text-muted-foreground size-4" />
					{/if}
				</button>
				{#if inviteOpen}
					<div class="space-y-2 border-t px-2.5 pt-2 pb-2.5">
						<p class="text-muted-foreground text-[11px]">输入对方用户名发送好友邀请</p>
						<div class="flex gap-2">
							<Input
								bind:value={inviteUsername}
								placeholder="用户名"
								class="h-8 flex-1"
								autocomplete="off"
								onkeydown={(e) => {
									if (e.key === 'Enter') {
										e.preventDefault();
										void handleInvite();
									}
								}}
							/>
							<Button
								size="sm"
								class="h-8 shrink-0"
								disabled={inviteBusy || !inviteUsername.trim()}
								onclick={() => void handleInvite()}
							>
								{#if inviteBusy}
									<LoaderCircle class="size-3.5 animate-spin" />
								{:else}
									邀请
								{/if}
							</Button>
						</div>
						{#if inviteError}
							<p class="text-destructive text-xs">{inviteError}</p>
						{/if}
						{#if inviteOk}
							<p class="text-xs text-emerald-600 dark:text-emerald-400">{inviteOk}</p>
						{/if}
					</div>
				{/if}
			</div>
		{:else}
			<!-- Search my groups -->
			<div class="relative">
				<Search
					class="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2"
				/>
				<Input
					bind:value={groupFilter}
					placeholder="搜索我的群…"
					class="h-9 pl-8"
					autocomplete="off"
				/>
			</div>

			<!-- Create group (collapsible) -->
			<div class="rounded-lg border border-border/60 bg-muted/15">
				<button
					type="button"
					class="hover:bg-muted/40 flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition-colors"
					onclick={() => (createGroupOpen = !createGroupOpen)}
				>
					<div class="bg-primary/10 text-primary flex size-7 items-center justify-center rounded-md">
						<Plus class="size-3.5" />
					</div>
					<span class="min-w-0 flex-1 font-medium">创建群聊</span>
					{#if createGroupOpen}
						<ChevronDown class="text-muted-foreground size-4" />
					{:else}
						<ChevronRight class="text-muted-foreground size-4" />
					{/if}
				</button>
				{#if createGroupOpen}
					<div class="space-y-2 border-t px-2.5 pt-2 pb-2.5">
						<div class="space-y-1">
							<label class="text-foreground text-xs font-medium" for="create-group-name">
								群名称 <span class="text-destructive">*</span>
							</label>
							<Input
								id="create-group-name"
								bind:value={createName}
								placeholder="例如：周末篮球局"
								maxlength={40}
								autocomplete="off"
								class="h-8"
								onkeydown={(e) => {
									if (e.key === 'Enter' && canCreate) {
										e.preventDefault();
										void handleCreate();
									}
								}}
							/>
							<p class="text-muted-foreground text-[10px]">2–40 字，成员可见的显示名</p>
						</div>
						<div class="space-y-1">
							<label class="text-muted-foreground text-xs font-medium" for="create-group-id">
								自定义群 ID <span class="font-normal">(可选)</span>
							</label>
							<Input
								id="create-group-id"
								bind:value={createId}
								placeholder="留空自动生成"
								maxlength={64}
								autocomplete="off"
								class="h-8 {createIdHint ? 'border-destructive/60' : ''}"
							/>
							{#if createIdHint}
								<p class="text-destructive text-[10px]">{createIdHint}</p>
							{:else}
								<p class="text-muted-foreground text-[10px]">3–64 位字母数字与 _ -</p>
							{/if}
						</div>
						<Button
							size="sm"
							class="h-8 w-full"
							disabled={!canCreate}
							onclick={() => void handleCreate()}
						>
							{#if createBusy}
								<LoaderCircle class="size-3.5 animate-spin" />
								创建中…
							{:else}
								<Plus class="size-3.5" />
								创建并进入
							{/if}
						</Button>
						{#if createError}
							<p class="text-destructive text-xs">{createError}</p>
						{/if}
					</div>
				{/if}
			</div>

			<!-- Join group (collapsible, autocomplete) -->
			<div class="rounded-lg border border-border/60 bg-muted/15">
				<button
					type="button"
					class="hover:bg-muted/40 flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition-colors"
					onclick={() => (joinGroupOpen = !joinGroupOpen)}
				>
					<div
						class="flex size-7 items-center justify-center rounded-md bg-emerald-500/15 text-emerald-700 dark:text-emerald-300"
					>
						<UserPlus class="size-3.5" />
					</div>
					<span class="min-w-0 flex-1 font-medium">加入群聊</span>
					{#if joinGroupOpen}
						<ChevronDown class="text-muted-foreground size-4" />
					{:else}
						<ChevronRight class="text-muted-foreground size-4" />
					{/if}
				</button>
				{#if joinGroupOpen}
					<div class="space-y-2 border-t px-2.5 pt-2 pb-2.5">
						<p class="text-muted-foreground text-[11px]">输入群名或 ID，从列表选择后加入</p>
						<div class="relative">
							<div class="flex gap-2">
								<div class="relative min-w-0 flex-1">
									<Input
										bind:value={joinGroupInput}
										placeholder="搜索群名或 ID…"
										class="h-8 w-full pr-8"
										autocomplete="off"
										oninput={() => scheduleJoinSearch(joinGroupInput)}
										onfocus={() => {
											if (joinGroupInput.trim()) scheduleJoinSearch(joinGroupInput);
										}}
										onkeydown={onJoinKeydown}
										onblur={() => {
											setTimeout(() => {
												joinSuggestOpen = false;
											}, 150);
										}}
									/>
									{#if joinSearchBusy}
										<span
											class="text-muted-foreground pointer-events-none absolute top-1/2 right-2 -translate-y-1/2"
										>
											<LoaderCircle class="size-3.5 animate-spin" />
										</span>
									{/if}
								</div>
								<Button size="sm" class="h-8 shrink-0" onclick={handleJoin} title="加入选中的群">
									加入
								</Button>
							</div>
							{#if joinSuggestOpen && joinSuggestions.length > 0}
								<ul
									class="border-border bg-popover text-popover-foreground absolute z-50 mt-1 max-h-56 w-full overflow-auto rounded-md border py-1 shadow-lg"
									role="listbox"
								>
									{#each joinSuggestions as s, i (s.id)}
										<li role="option" aria-selected={i === joinHighlight}>
											<button
												type="button"
												class="flex w-full items-start gap-2 px-2.5 py-2 text-left text-sm transition-colors
													{i === joinHighlight ? 'bg-accent' : 'hover:bg-muted/80'}"
												onmousedown={(e) => {
													e.preventDefault();
													pickJoinSuggestion(s);
												}}
												onmouseenter={() => (joinHighlight = i)}
											>
												<div
													class="bg-primary/10 text-primary mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md"
												>
													<Hash class="size-3.5" />
												</div>
												<div class="min-w-0 flex-1">
													<p class="truncate font-medium">{s.name || s.id}</p>
													<p class="text-muted-foreground truncate text-[11px]">
														id: {s.id}
														{#if s.member_count != null}
															· {s.member_count} 人
														{/if}
														{#if s.owner_username}
															· 群主 {s.owner_username}
														{/if}
													</p>
												</div>
												{#if s.is_member}
													<Badge variant="secondary" class="shrink-0 text-[10px]">已加入</Badge>
												{:else}
													<Badge
														variant="outline"
														class="shrink-0 border-emerald-500/40 text-[10px] text-emerald-700 dark:text-emerald-300"
													>
														可加入
													</Badge>
												{/if}
											</button>
										</li>
									{/each}
								</ul>
							{/if}
							{#if joinSearchEmpty && joinGroupInput.trim().length > 0 && !joinSearchBusy}
								<p class="text-muted-foreground mt-1 text-[11px]">
									未找到匹配群，可直接输入完整 ID 后点加入
								</p>
							{/if}
						</div>
					</div>
				{/if}
			</div>
		{/if}
	</div>

	<Separator />

	{#if chatMode === 'private'}
		<div class="flex min-h-0 flex-1 flex-col">
			<!-- Incoming friend requests -->
			{#if incomingRequests.length > 0}
				<div class="border-b px-3 py-2">
					<p class="text-muted-foreground mb-1.5 text-[11px] font-medium tracking-wide uppercase">
						好友申请 · {incomingRequests.length}
					</p>
					<ul class="space-y-1.5">
						{#each incomingRequests as req (req.id)}
							<li
								class="bg-muted/50 flex items-center gap-2 rounded-lg border border-border/50 px-2 py-2"
							>
								<div class="relative shrink-0">
									<UserAvatar
										class="size-9"
										name={req.from_username || req.from_user_id}
										userId={req.from_user_id}
										src={avatarUrl(req.from_user_id)}
									/>
								</div>
								<div class="min-w-0 flex-1">
									<p class="truncate text-sm font-medium">
										{req.from_username || req.from_user_id}
									</p>
									<p class="text-muted-foreground text-[11px]">请求添加你为好友</p>
								</div>
								<Button
									size="sm"
									class="h-7 px-2"
									title="同意"
									onclick={() => void onAcceptRequest(req.id)}
								>
									<Check class="size-3.5" />
								</Button>
								<Button
									variant="outline"
									size="sm"
									class="h-7 px-2"
									title="拒绝"
									onclick={() => void onRejectRequest(req.id)}
								>
									<X class="size-3.5" />
								</Button>
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			<!-- Friend list header -->
			<div class="flex items-center justify-between gap-2 px-3 py-2">
				<div class="min-w-0">
					<p class="text-sm font-semibold">会话</p>
					<p class="text-muted-foreground text-[11px]">
						{friends.length} 位好友
						{#if onlineFriendCount > 0}
							· <span class="text-emerald-600 dark:text-emerald-400">{onlineFriendCount} 在线</span>
						{/if}
					</p>
				</div>
				<Button
					variant="ghost"
					size="icon-xs"
					onclick={() => {
						onRefreshFriends();
						onRefreshOnline();
					}}
					aria-label="刷新好友列表"
					title="刷新"
				>
					<RefreshCw class="size-3.5" />
				</Button>
			</div>

			<ScrollArea.Root class="min-h-0 flex-1">
				<ul class="space-y-0.5 px-2 pb-2">
					{#each sortedFriends as u (u.user_id)}
						{@const preview = lastPreviews[`private:${u.user_id}`]}
						{@const active = targetUser === u.user_id}
						{@const unread = !!unreadPeers[u.user_id]}
						<li>
							<div
								class="group flex items-center gap-0.5 rounded-xl transition-colors
									{active ? 'bg-sidebar-accent' : 'hover:bg-sidebar-accent/70'}
									{unread && !active ? 'bg-amber-500/10 ring-1 ring-amber-400/30' : ''}"
							>
								<button
									type="button"
									class="flex min-w-0 flex-1 items-center gap-2.5 px-2 py-2.5 text-left"
									onclick={() => onSelectUser(u.user_id, u.username)}
								>
									<div class="relative shrink-0">
										<UserAvatar
											class="size-10"
											name={u.username || u.user_id}
											userId={u.user_id}
											src={avatarUrl(u.user_id)}
										/>
										<span
											class="border-background absolute right-0 bottom-0 size-2.5 rounded-full border-2
												{u.online ? 'bg-emerald-500' : 'bg-muted-foreground/35'}"
											title={u.online ? '在线' : '离线'}
										></span>
									</div>
									<div class="min-w-0 flex-1">
										<div class="flex items-center gap-1.5">
											<span class="truncate text-sm font-medium">
												{u.username || u.user_id}
											</span>
											{#if unread}
												<span class="bg-amber-500 size-1.5 shrink-0 rounded-full"></span>
											{/if}
											<span class="text-muted-foreground ml-auto shrink-0 text-[10px]">
												{formatRelativeTime(preview?.ts)}
											</span>
										</div>
										<p
											class="truncate text-[11px]
												{unread ? 'font-medium text-foreground/80' : 'text-muted-foreground'}"
										>
											{#if unread}
												[未读]
											{/if}
											{preview?.text || (u.online ? '在线' : '离线')}
										</p>
									</div>
								</button>

								<DropdownMenu.Root>
									<DropdownMenu.Trigger
										class="text-muted-foreground hover:bg-muted mr-1 inline-flex size-7 shrink-0 items-center justify-center rounded-md opacity-0 transition-opacity group-hover:opacity-100 data-[state=open]:opacity-100"
										title="更多操作"
										onclick={(e: MouseEvent) => e.stopPropagation()}
									>
										<MoreHorizontal class="size-4" />
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="end" class="w-44">
										<DropdownMenu.Label class="truncate">
											{u.username || u.user_id}
										</DropdownMenu.Label>
										<DropdownMenu.Separator />
										{#if onCallUser}
											<DropdownMenu.Item
												disabled={callDisabled}
												onclick={() => void onCallUser(u.user_id, u.username, 'audio')}
											>
												<Phone class="size-3.5" />
												语音通话
											</DropdownMenu.Item>
											<DropdownMenu.Item
												disabled={callDisabled}
												onclick={() => void onCallUser(u.user_id, u.username, 'video')}
											>
												<Video class="size-3.5" />
												视讯通话
											</DropdownMenu.Item>
											<DropdownMenu.Separator />
										{/if}
										<DropdownMenu.Item
											variant="destructive"
											onclick={async () => {
												if (
													await confirmDialog({
														title: '解除好友',
														message: `确定解除与 ${u.username || u.user_id} 的好友关系？`,
														confirmText: '解除',
														danger: true
													})
												) {
													void onRemoveFriend(u.user_id);
												}
											}}
										>
											<UserMinus class="size-3.5" />
											解除好友
										</DropdownMenu.Item>
										<DropdownMenu.Item
											variant="destructive"
											onclick={async () => {
												if (
													await confirmDialog({
														title: '拉黑用户',
														message: `拉黑 ${u.username || u.user_id}？将解除好友，且无法互相邀请/私聊。`,
														confirmText: '拉黑',
														danger: true
													})
												) {
													void onBlockUser({ user_id: u.user_id });
												}
											}}
										>
											<Ban class="size-3.5" />
											拉黑
										</DropdownMenu.Item>
									</DropdownMenu.Content>
								</DropdownMenu.Root>
							</div>
						</li>
					{:else}
						<li class="text-muted-foreground flex flex-col items-center gap-2 px-3 py-10 text-center">
							<div class="bg-muted flex size-12 items-center justify-center rounded-full">
								<Users class="size-5 opacity-50" />
							</div>
							{#if friendFilter.trim()}
								<p class="text-sm">没有匹配的好友</p>
								<p class="text-xs">试试其他关键词</p>
							{:else}
								<p class="text-sm font-medium">还没有好友</p>
								<p class="text-xs">点上方「添加好友」邀请对方</p>
							{/if}
						</li>
					{/each}
				</ul>

				<!-- Online strangers (not friends) -->
				{#if othersOnline.length > 0}
					<div class="border-t px-2 pt-2 pb-1">
						<button
							type="button"
							class="text-muted-foreground hover:text-foreground flex w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-left text-[11px] font-medium tracking-wide uppercase"
							onclick={() => (onlineOpen = !onlineOpen)}
						>
							{#if onlineOpen}
								<ChevronDown class="size-3.5" />
							{:else}
								<ChevronRight class="size-3.5" />
							{/if}
							在线陌生人 · {othersOnline.length}
						</button>
						{#if onlineOpen}
							<p class="text-muted-foreground/80 px-2 pb-1 text-[10px]">
								可邀请为好友后私聊
							</p>
							<ul class="space-y-0.5 px-1 pb-2">
								{#each othersOnline as u (u.user_id)}
									<li class="flex items-center gap-1 rounded-lg px-1.5 py-1.5 hover:bg-sidebar-accent/60">
										<div class="relative shrink-0">
											<UserAvatar
												class="size-8"
												name={u.username || u.user_id}
												userId={u.user_id}
												src={avatarUrl(u.user_id)}
											/>
											<span
												class="border-background absolute right-0 bottom-0 size-2 rounded-full border-2 bg-emerald-500"
											></span>
										</div>
										<span class="min-w-0 flex-1 truncate text-sm">
											{u.username || u.user_id}
										</span>
										<Button
											variant="outline"
											size="sm"
											class="h-7 gap-1 px-2 text-xs"
											onclick={() => {
												inviteOpen = true;
												inviteUsername = u.username || u.user_id;
											}}
										>
											<UserPlus class="size-3" />
											邀请
										</Button>
									</li>
								{/each}
							</ul>
						{/if}
					</div>
				{/if}

				<!-- Blacklist -->
				{#if blacklist.length > 0}
					<div class="border-t px-2 pt-2 pb-3">
						<button
							type="button"
							class="text-muted-foreground hover:text-foreground flex w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-left text-[11px] font-medium tracking-wide uppercase"
							onclick={() => (blacklistOpen = !blacklistOpen)}
						>
							{#if blacklistOpen}
								<ChevronDown class="size-3.5" />
							{:else}
								<ChevronRight class="size-3.5" />
							{/if}
							黑名单 · {blacklist.length}
						</button>
						{#if blacklistOpen}
							<ul class="space-y-0.5 px-1 pt-1">
								{#each blacklist as u (u.user_id)}
									<li class="flex items-center gap-2 rounded-lg px-2 py-1.5">
										<Ban class="text-muted-foreground size-3.5 shrink-0 opacity-70" />
										<span class="text-muted-foreground min-w-0 flex-1 truncate text-sm">
											{u.username || u.user_id}
										</span>
										<Button
											variant="ghost"
											size="sm"
											class="h-7 px-2 text-xs"
											title="取消拉黑"
											onclick={() => void onUnblockUser(u.user_id)}
										>
											<ShieldOff class="size-3.5" />
											解除
										</Button>
									</li>
								{/each}
							</ul>
						{/if}
					</div>
				{/if}
			</ScrollArea.Root>
		</div>
	{:else}
		<div class="flex min-h-0 flex-1 flex-col">
			<div class="flex items-center justify-between gap-2 px-3 py-2">
				<div class="min-w-0">
					<p class="text-sm font-semibold">我的群聊</p>
					<p class="text-muted-foreground text-[11px]">
						{joinedGroups.length} 个群
						{#if ownerGroupCount > 0}
							· {ownerGroupCount} 个我创建
						{/if}
					</p>
				</div>
				{#if onRefreshGroups}
					<Button
						variant="ghost"
						size="icon-xs"
						onclick={() => onRefreshGroups()}
						aria-label="刷新群列表"
						title="刷新"
					>
						<RefreshCw class="size-3.5" />
					</Button>
				{/if}
			</div>

			<ScrollArea.Root class="min-h-0 flex-1">
				<ul class="space-y-0.5 px-2 pb-4">
					{#each sortedGroups as g (g)}
						{@const preview = lastPreviews[`group:${g}`]}
						{@const active = groupId === g}
						{@const unread = !!unreadGroups[g]}
						{@const meeting = activeMeetings[g]}
						{@const owner = isOwner(g)}
						{@const members = groupMeta[g]?.member_count}
						{@const meta = groupMeta[g]}
						{@const iconSrc =
							meta?.avatar || meta?.avatar_rev
								? groupAvatarUrl(g, meta?.avatar_rev)
								: ''}
						<li>
							<div
								class="group flex items-center gap-0.5 rounded-xl transition-colors
									{active ? 'bg-sidebar-accent' : 'hover:bg-sidebar-accent/70'}
									{unread && !active ? 'bg-amber-500/10 ring-1 ring-amber-400/30' : ''}"
							>
								<button
									type="button"
									class="flex min-w-0 flex-1 items-center gap-2.5 px-2 py-2.5 text-left"
									onclick={() => onSelectGroup(g)}
									title={g}
								>
									<div
										class="relative flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-xl text-sm font-semibold text-white shadow-sm"
										style="background: hsl({groupHue(g)} 55% 42%)"
									>
										{#if iconSrc}
											<img
												src={iconSrc}
												alt=""
												class="absolute inset-0 size-full object-cover"
												onerror={(e) => {
													(e.currentTarget as HTMLImageElement).style.display = 'none';
												}}
											/>
										{/if}
										<span class="relative z-0">{groupInitial(g)}</span>
										{#if meeting}
											<span
												class="border-background absolute -right-0.5 -bottom-0.5 z-10 size-2.5 rounded-full border-2 bg-emerald-500"
												title="会议进行中"
											></span>
										{/if}
									</div>
									<div class="min-w-0 flex-1">
										<div class="flex items-center gap-1.5">
											<span class="truncate text-sm font-medium">{groupLabel(g)}</span>
											{#if owner}
												<Crown class="size-3 shrink-0 text-amber-500" title="群主" />
											{:else if meta?.role === 'admin'}
												<span
													class="shrink-0 rounded bg-sky-500/15 px-1 text-[10px] font-medium text-sky-700 dark:text-sky-300"
													title="管理者"
												>管</span>
											{/if}
											{#if unread}
												<span class="bg-amber-500 size-1.5 shrink-0 rounded-full"></span>
											{/if}
											<span class="text-muted-foreground ml-auto shrink-0 text-[10px]">
												{formatRelativeTime(preview?.ts)}
											</span>
										</div>
										<p
											class="truncate text-[11px]
												{unread ? 'font-medium text-foreground/80' : 'text-muted-foreground'}"
										>
											{#if meeting}
												<span class="text-emerald-600 dark:text-emerald-400">
													[{meeting.media === 'video' ? '视讯' : '语音'}会议中]
												</span>
											{:else if unread}
												[未读]
											{/if}
											{preview?.text ||
												(typeof members === 'number' ? `${members} 人` : `id: ${g}`)}
										</p>
									</div>
								</button>

								<DropdownMenu.Root>
									<DropdownMenu.Trigger
										class="text-muted-foreground hover:bg-muted mr-1 inline-flex size-7 shrink-0 items-center justify-center rounded-md opacity-0 transition-opacity group-hover:opacity-100 data-[state=open]:opacity-100"
										title="更多操作"
										onclick={(e: MouseEvent) => e.stopPropagation()}
									>
										<MoreHorizontal class="size-4" />
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="end" class="w-44">
										<DropdownMenu.Label class="truncate">{groupLabel(g)}</DropdownMenu.Label>
										<DropdownMenu.Separator />
										<DropdownMenu.Item onclick={() => onSelectGroup(g)}>
											<Hash class="size-3.5" />
											进入群聊
										</DropdownMenu.Item>
										{#if onOpenGroupSettings}
											<DropdownMenu.Item
												onclick={() => {
													onSelectGroup(g);
													onOpenGroupSettings(g);
												}}
											>
												<Settings class="size-3.5" />
												群配置
											</DropdownMenu.Item>
										{/if}
										{#if owner}
											<DropdownMenu.Item
												variant="destructive"
												onclick={async () => {
													if (
														await confirmDialog({
															title: '解散群聊',
															message: `解散群「${groupLabel(g)}」？所有成员将被移除。`,
															confirmText: '解散',
															danger: true
														})
													) {
														void onDissolveGroup(g);
													}
												}}
											>
												<Trash2 class="size-3.5" />
												解散群
											</DropdownMenu.Item>
										{:else}
											<DropdownMenu.Item
												variant="destructive"
												onclick={() => onLeaveGroup(g)}
											>
												<LogOut class="size-3.5" />
												退出群
											</DropdownMenu.Item>
										{/if}
									</DropdownMenu.Content>
								</DropdownMenu.Root>
							</div>
						</li>
					{:else}
						<li class="text-muted-foreground flex flex-col items-center gap-2 px-3 py-10 text-center">
							<div class="bg-muted flex size-12 items-center justify-center rounded-full">
								<Hash class="size-5 opacity-50" />
							</div>
							{#if groupFilter.trim()}
								<p class="text-sm">没有匹配的群</p>
								<p class="text-xs">试试其他关键词</p>
							{:else}
								<p class="text-sm font-medium">还没有群聊</p>
								<p class="text-xs">上方可创建群，或搜索加入已有群</p>
							{/if}
						</li>
					{/each}
				</ul>
			</ScrollArea.Root>
		</div>
	{/if}
</aside>
