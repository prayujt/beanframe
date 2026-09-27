import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
export default {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({ fallback: '200.html' }),
    csp: {
      mode: 'hash',
      directives: {
        'default-src': ['self'],
        'script-src': ['self'],
        'img-src': ['self', 'https:', 'http:'],
        // Replaced by the Go server with a fresh nonce on every HTML response.
        'style-src': ['self', 'nonce-__LEDGER_STYLE_NONCE__'],
        // SvelteKit's accessible route announcer uses inline style attributes.
        'style-src-attr': ['unsafe-inline'],
        'object-src': ['none'],
        'base-uri': ['self']
      }
    }
  }
};
