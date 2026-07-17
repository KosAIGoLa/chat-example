/**
 * Global toast + alert/confirm dialogs (replaces window.alert / confirm).
 */

export type ToastKind = 'info' | 'success' | 'error' | 'warning';

export interface ToastItem {
	id: number;
	kind: ToastKind;
	title?: string;
	message: string;
	duration: number;
}

export interface ConfirmOptions {
	title?: string;
	message: string;
	confirmText?: string;
	cancelText?: string;
	danger?: boolean;
}

export interface AlertOptions {
	title?: string;
	message: string;
	okText?: string;
	/** Visual tone of the OK action / icon. */
	kind?: ToastKind;
}

let toastSeq = 0;
let toasts = $state<ToastItem[]>([]);

let confirmOpen = $state(false);
let confirmOpts = $state<ConfirmOptions>({ message: '' });
let confirmResolver: ((ok: boolean) => void) | null = null;

let alertOpen = $state(false);
let alertOpts = $state<AlertOptions>({ message: '' });
let alertResolver: (() => void) | null = null;

export function getToasts(): ToastItem[] {
	return toasts;
}

export function isConfirmOpen(): boolean {
	return confirmOpen;
}

export function getConfirmOptions(): ConfirmOptions {
	return confirmOpts;
}

export function isAlertOpen(): boolean {
	return alertOpen;
}

export function getAlertOptions(): AlertOptions {
	return alertOpts;
}

export function toast(
	message: string,
	opts: { kind?: ToastKind; title?: string; duration?: number } = {}
) {
	const id = ++toastSeq;
	const item: ToastItem = {
		id,
		kind: opts.kind ?? 'info',
		title: opts.title,
		message,
		duration: opts.duration ?? 3200
	};
	toasts = [...toasts, item];
	if (item.duration > 0) {
		setTimeout(() => dismissToast(id), item.duration);
	}
	return id;
}

export function toastError(message: string, title = '出错了') {
	return toast(message, { kind: 'error', title, duration: 4500 });
}

export function toastSuccess(message: string, title?: string) {
	return toast(message, { kind: 'success', title, duration: 2800 });
}

export function toastInfo(message: string, title?: string) {
	return toast(message, { kind: 'info', title });
}

export function dismissToast(id: number) {
	toasts = toasts.filter((t) => t.id !== id);
}

/** Promise-based alert modal (replaces window.alert). Single OK button. */
export function alertDialog(opts: AlertOptions | string): Promise<void> {
	const o: AlertOptions = typeof opts === 'string' ? { message: opts } : opts;
	if (alertResolver) {
		alertResolver();
		alertResolver = null;
	}
	alertOpts = {
		title: o.title ?? '提示',
		message: o.message,
		okText: o.okText ?? '知道了',
		kind: o.kind ?? 'info'
	};
	alertOpen = true;
	return new Promise<void>((resolve) => {
		alertResolver = resolve;
	});
}

/** Convenience: error alert modal. */
export function alertError(message: string, title = '出错了'): Promise<void> {
	return alertDialog({ title, message, kind: 'error', okText: '知道了' });
}

/** Promise-based confirm modal (replaces window.confirm). */
export function confirmDialog(opts: ConfirmOptions | string): Promise<boolean> {
	const o: ConfirmOptions = typeof opts === 'string' ? { message: opts } : opts;
	// Close any previous pending confirm as cancel.
	if (confirmResolver) {
		confirmResolver(false);
		confirmResolver = null;
	}
	confirmOpts = {
		title: o.title ?? '请确认',
		message: o.message,
		confirmText: o.confirmText ?? '确定',
		cancelText: o.cancelText ?? '取消',
		danger: o.danger ?? false
	};
	confirmOpen = true;
	return new Promise<boolean>((resolve) => {
		confirmResolver = resolve;
	});
}

export function resolveConfirm(ok: boolean) {
	confirmOpen = false;
	const r = confirmResolver;
	confirmResolver = null;
	r?.(ok);
}

export function resolveAlert() {
	alertOpen = false;
	const r = alertResolver;
	alertResolver = null;
	r?.();
}
