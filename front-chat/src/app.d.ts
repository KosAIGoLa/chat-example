// See https://svelte.dev/docs/kit/types#app.d.ts
// Pull in Svelte ambient types ($state, $props, declare module '*.svelte', …)
// so plain TypeScript / JetBrains language service can resolve .svelte imports.
/// <reference types="svelte" />

declare global {
	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

export {};
