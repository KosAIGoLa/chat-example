import type { UserInfo } from './types';

interface AuthState {
	token: string | null;
	user: UserInfo | null;
}

function createAuthStore() {
	const state = $state<AuthState>({
		token: null,
		user: null
	});

	if (typeof window !== 'undefined') {
		const savedToken = localStorage.getItem('token');
		const savedUser = localStorage.getItem('user');
		state.token = savedToken;
		state.user = savedUser ? JSON.parse(savedUser) : null;
	}

	return {
		get token() {
			return state.token;
		},
		get user() {
			return state.user;
		},
		get isAuthenticated() {
			return !!state.token;
		},
		setAuth: (token: string, user: UserInfo) => {
			state.token = token;
			state.user = user;
			localStorage.setItem('token', token);
			localStorage.setItem('user', JSON.stringify(user));
		},
		updateUser: (user: UserInfo) => {
			state.user = user;
			localStorage.setItem('user', JSON.stringify(user));
		},
		logout: () => {
			state.token = null;
			state.user = null;
			localStorage.removeItem('token');
			localStorage.removeItem('user');
		}
	};
}

export const auth = createAuthStore();
