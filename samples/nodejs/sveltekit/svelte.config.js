import adapter from '@sveltejs/adapter-node';
import preprocess from 'svelte-preprocess';

export default {
  preprocess: preprocess(),

  kit: {
    adapter: adapter(),
  }
};
