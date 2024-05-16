import { connectToDatabase } from '$lib/db';
import fetch from 'node-fetch';
import { ObjectId } from 'mongodb';
import { json } from '@sveltejs/kit'; // Import the json utility

export async function GET({ url }) {
  const query = url.searchParams.get('query');
  if (!query) {
    return json({ error: 'Query parameter is required' }, { status: 400 });
  }

  const { db } = await connectToDatabase();

  // Check if the search query is already cached
  const cachedSearch = await db.collection('searches').findOne({ query });

  if (cachedSearch) {
    // Fetch cached results from the Songs collection
    const songIds = cachedSearch.results.map(id => new ObjectId(id));
    const songs = await db.collection('songs').find({ _id: { $in: songIds } }).toArray();
    return json({ songs });
  }

  // If not cached, fetch from MusicBrainz API
  const response = await fetch(`https://musicbrainz.org/ws/2/recording?query=${query}&fmt=json`);
  const data = await response.json();

  // Ensure the data is in the expected format
  if (!data.recordings || !Array.isArray(data.recordings)) {
    return json({ error: 'Invalid data format from MusicBrainz API' }, { status: 500 });
  }

  const songs = data.recordings.map(recording => ({
    title: recording.title,
    artist: recording['artist-credit']?.[0]?.name || 'Unknown artist',
    album: recording.releases?.[0]?.title || 'Unknown album',
    musicbrainz_id: recording.id,
    data: recording
  }));

  // Save the songs in the Songs collection
  const result = await db.collection('songs').insertMany(songs);
  const songIds = Object.values(result.insertedIds);

  // Cache the search query and results in the Searches collection
  await db.collection('searches').insertOne({ query, results: songIds, timestamp: new Date() });

  return json({ songs });
}
