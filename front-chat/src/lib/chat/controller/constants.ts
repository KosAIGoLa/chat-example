/** Reconnect backoff: 1s → 2s → 4s … capped at 30s. */
export const RECONNECT_BASE_MS = 1000;
export const RECONNECT_MAX_MS = 30_000;
export const RECONNECT_MAX_ATTEMPTS = 50;

/**
 * Application-level heartbeat (encrypted JSON frames).
 * Server read deadline is 60s; ping well under that and fail if no pong.
 */
export const HEARTBEAT_INTERVAL_MS = 20_000;
export const HEARTBEAT_TIMEOUT_MS = 10_000;

export const TYPING_TTL_MS = 3000;
export const TYPING_PING_MS = 1500;
export const TYPING_IDLE_MS = 1500;

export const HISTORY_PAGE = 50;

export const JOINED_GROUPS_KEY = 'joined_groups';
