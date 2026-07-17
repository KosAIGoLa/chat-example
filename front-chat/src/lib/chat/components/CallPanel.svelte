<script lang="ts">
	import type { CallController } from '../call.svelte';
	import { Button } from '$lib/components/ui/button';
	import Phone from '@lucide/svelte/icons/phone';
	import PhoneOff from '@lucide/svelte/icons/phone-off';
	import Mic from '@lucide/svelte/icons/mic';
	import MicOff from '@lucide/svelte/icons/mic-off';
	import Video from '@lucide/svelte/icons/video';
	import VideoOff from '@lucide/svelte/icons/video-off';
	import Users from '@lucide/svelte/icons/users';
	import LogOut from '@lucide/svelte/icons/log-out';

	interface Props {
		call: CallController;
	}

	let { call }: Props = $props();

	let localVideo: HTMLVideoElement | undefined = $state();
	let remoteVideoEls = $state<Record<string, HTMLVideoElement | undefined>>({});

	const isVideo = $derived(call.isVideo);
	const isMeeting = $derived(call.callType === 'group');
	const kindLabel = $derived(isVideo ? '视讯' : '语音');

	const title = $derived(
		isMeeting
			? `${kindLabel}会议 · ${call.groupId || call.roomName}`
			: `${kindLabel}通话 · ${call.peerName || call.peerId}`
	);

	const statusLabel = $derived(
		call.phase === 'outgoing'
			? call.participants.length === 0
				? `正在${isVideo ? '视讯' : '语音'}呼叫…`
				: '对方已接通'
			: call.phase === 'incoming'
				? `${kindLabel}来电…`
				: call.phase === 'connecting'
					? isMeeting
						? '正在进入会议…'
						: '正在连接媒体…'
					: call.phase === 'connected'
						? isMeeting
							? isVideo
								? '视讯会议中 · 可继续群聊'
								: '语音会议中 · 可继续群聊'
							: isVideo
								? '视讯通话中'
								: '语音通话中'
						: call.phase === 'ended'
							? isMeeting
								? '已离开会议'
								: '通话已结束'
							: ''
	);

	$effect(() => {
		if (!isVideo) return;
		call.setLocalVideoEl(localVideo ?? null);
		return () => call.setLocalVideoEl(null);
	});

	$effect(() => {
		if (!isVideo) return;
		const ids = call.participants.map((p) => p.identity);
		for (const id of ids) {
			call.setRemoteVideoEl(id, remoteVideoEls[id] ?? null);
		}
		return () => {
			for (const id of ids) call.setRemoteVideoEl(id, null);
		};
	});
</script>

{#if call.phase !== 'idle'}
	<div
		class="border-border bg-background/95 fixed inset-x-0 bottom-0 z-50 border-t p-4 shadow-2xl backdrop-blur md:inset-x-auto md:right-4 md:bottom-4 md:w-[min(28rem,calc(100vw-2rem))] md:rounded-2xl md:border"
	>
		<div class="mb-3 flex items-start justify-between gap-2">
			<div class="min-w-0">
				<p class="truncate text-sm font-semibold">{title}</p>
				<p class="text-muted-foreground text-xs">{statusLabel}</p>
				{#if call.error}
					<p class="text-destructive mt-1 text-xs">{call.error}</p>
				{/if}
			</div>
			{#if isMeeting}
				<span class="text-muted-foreground inline-flex items-center gap-1 text-xs">
					<Users class="size-3.5" />
					{call.participants.length + 1}
				</span>
			{/if}
		</div>

		{#if call.phase === 'incoming'}
			<!-- Private ring only -->
			<div class="bg-muted/50 mb-3 flex items-center gap-3 rounded-xl px-3 py-3">
				<div
					class="bg-primary/15 text-primary flex size-12 items-center justify-center rounded-full"
				>
					{#if isVideo}
						<Video class="size-6" />
					{:else}
						<Phone class="size-6" />
					{/if}
				</div>
				<div class="min-w-0 flex-1">
					<p class="truncate font-medium">{call.peerName || call.peerId}</p>
					<p class="text-muted-foreground text-xs">
						{isVideo ? '邀请你视讯通话' : '邀请你语音通话'}
					</p>
				</div>
			</div>
			<div class="flex gap-2">
				<Button class="flex-1" onclick={() => void call.acceptIncoming()}>
					{#if isVideo}
						<Video class="size-4" />
						接听视讯
					{:else}
						<Phone class="size-4" />
						接听
					{/if}
				</Button>
				<Button variant="destructive" class="flex-1" onclick={() => void call.rejectIncoming()}>
					<PhoneOff class="size-4" />
					拒绝
				</Button>
			</div>
		{:else if isVideo}
			<!-- 视讯：画面网格 -->
			<div class="mb-3 grid grid-cols-2 gap-2">
				<div class="bg-muted relative aspect-video overflow-hidden rounded-lg">
					<video
						bind:this={localVideo}
						class="h-full w-full object-cover"
						autoplay
						playsinline
						muted
					></video>
					<span
						class="bg-background/70 absolute bottom-1 left-1 rounded px-1.5 py-0.5 text-[10px]"
						>我</span
					>
				</div>
				{#if call.participants.length === 0}
					<div
						class="bg-muted text-muted-foreground flex aspect-video items-center justify-center rounded-lg text-xs"
					>
						{isMeeting ? '等待成员加入…' : '等待对方加入…'}
					</div>
				{:else}
					{#each call.participants as p (p.identity)}
						<div class="bg-muted relative aspect-video overflow-hidden rounded-lg">
							<video
								bind:this={remoteVideoEls[p.identity]}
								class="h-full w-full object-cover"
								autoplay
								playsinline
							></video>
							<span
								class="bg-background/70 absolute bottom-1 left-1 max-w-[90%] truncate rounded px-1.5 py-0.5 text-[10px]"
								>{p.name}</span
							>
						</div>
					{/each}
				{/if}
			</div>
		{:else}
			<!-- 语音：大头像/占位，无视频 -->
			<div
				class="bg-muted mb-3 flex flex-col items-center justify-center gap-2 rounded-xl px-4 py-8"
			>
				<div
					class="bg-primary/15 text-primary flex size-16 items-center justify-center rounded-full"
				>
					{#if isMeeting}
						<Users class="size-8" />
					{:else}
						<Phone class="size-8" />
					{/if}
				</div>
				<p class="text-sm font-medium">
					{#if isMeeting}
						{call.phase === 'connecting' ? '正在进入会议…' : '语音会议中'}
					{:else if call.phase === 'outgoing' && call.participants.length === 0}
						等待对方接听…
					{:else}
						{call.peerName || '语音通话中'}
					{/if}
				</p>
				{#if call.participants.length > 0}
					<p class="text-muted-foreground text-xs">
						已连接 · {call.participants.map((p) => p.name).join('、')}
					</p>
				{:else if isMeeting}
					<p class="text-muted-foreground text-xs">成员可随时从群聊加入</p>
				{/if}
			</div>
		{/if}

		{#if call.phase !== 'incoming'}
			<div class="flex items-center justify-center gap-2">
				<Button
					variant={call.micEnabled ? 'secondary' : 'destructive'}
					size="icon"
					onclick={() => void call.toggleMic()}
					title={call.micEnabled ? '静音' : '开麦'}
				>
					{#if call.micEnabled}
						<Mic class="size-4" />
					{:else}
						<MicOff class="size-4" />
					{/if}
				</Button>
				{#if isVideo}
					<Button
						variant={call.camEnabled ? 'secondary' : 'destructive'}
						size="icon"
						onclick={() => void call.toggleCam()}
						title={call.camEnabled ? '关摄像头' : '开摄像头'}
					>
						{#if call.camEnabled}
							<Video class="size-4" />
						{:else}
							<VideoOff class="size-4" />
						{/if}
					</Button>
				{/if}
				{#if isMeeting}
					<Button
						variant="outline"
						size="icon"
						onclick={() => void call.hangup()}
						title="离开会议（其他人继续）"
					>
						<LogOut class="size-4" />
					</Button>
					<Button
						variant="destructive"
						size="icon"
						onclick={() => void call.endGroupMeeting()}
						title="结束会议（全员退出）"
					>
						<PhoneOff class="size-4" />
					</Button>
				{:else}
					<Button variant="destructive" size="icon" onclick={() => void call.hangup()} title="挂断">
						<PhoneOff class="size-4" />
					</Button>
				{/if}
			</div>
			{#if isMeeting}
				<p class="text-muted-foreground mt-2 text-center text-[10px]">
					离开 = 仅自己退出 · 结束 = 关闭整个会议
				</p>
			{/if}
		{/if}
	</div>
{/if}
