<script>
  import { onMount } from "svelte";
  import { writable } from "svelte/store";

  let query = "";
  let songs = writable([]);

  async function searchSongs() {
    const res = await fetch(`/api/songs?query=${query}`);
    const data = await res.json();
    songs.set(data.songs);
  }

  onMount(() => {
    // Optionally, perform an initial search
  });
</script>

<input bind:value={query} placeholder="Search for a song..." />
<button on:click={searchSongs}>Search</button>

<ul>
  {#each $songs as song}
    <li>{song.title} by {song.artist}</li>
  {/each}
</ul>
