import { sveltekit } from '@sveltejs/kit/vite';

export default {
  plugins: [sveltekit()],
  server: {
    host: '0.0.0.0',
    port: 3000
  }
};
