// src/routes/api/songs/+server.js
// this is server side rendering by sveltekit
import { json } from '@sveltejs/kit';

export async function GET({ url }) {
  const query = url.searchParams.get('query');

  if (!query) {
    return json({ error: 'Query parameter is required' }, { status: 400 });
  }

  const headers = {
    'User-Agent': 'sveltekit-music-app/1.0.0 (your-email@gmail.com)' // Replace with your app name and email
  };

  const response = await fetch(`https://musicbrainz.org/ws/2/recording/?query=${query}&fmt=json`, { headers });
  const data = await response.json();

  return json(data);
}
