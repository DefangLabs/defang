import { MongoClient } from 'mongodb';

const uri = process.env.MONGODB_URI || 'mongodb://localhost:27017/musicdb';
const client = new MongoClient(uri);

let cachedClient = null;
let cachedDb = null;

export async function connectToDatabase() {
  if (cachedClient && cachedDb) {
    return { client: cachedClient, db: cachedDb };
  }

  // Connect the client
  await client.connect();
  const db = client.db(); // Connects to the specified database in the URI

  // Ensure the collections exist (this step is optional because MongoDB will create collections on first use)
  await db.createCollection('songs').catch(() => {});
  await db.createCollection('searches').catch(() => {});

  cachedClient = client;
  cachedDb = db;
  return { client: cachedClient, db: cachedDb };
}
