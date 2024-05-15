<script>
  import { onMount } from "svelte";
  let query = "";
  let results = [];
  let error = "";

  async function searchSongs() {
    console.log("Searching for:", query);
    if (query.length > 2) {
      try {
        const response = await fetch(`/api/songs?query=${query}`);
        const data = await response.json();

        console.log("Response:", data);

        if (response.ok) {
          results = data.recordings || [];
          error = "";
        } else {
          results = [];
          error = data.error || "An error occurred";
        }
      } catch (err) {
        console.error("Fetch error:", err);
        results = [];
        error = "Failed to fetch data";
      }
    } else {
      results = [];
      error = "";
    }
  }
</script>

<main class="p-4">
  <h1 class="text-2xl mb-4">MusicBrainz Song Search</h1>
  <input
    type="text"
    bind:value={query}
    on:input={searchSongs}
    class="border p-2 w-full mb-4"
    placeholder="Search for songs..."
  />

  {#if error}
    <p class="text-red-500">{error}</p>
  {/if}

  {#if results.length > 0}
    <ul>
      {#each results as result}
        <li class="mb-2 p-2 border-b">
          <strong>{result.title}</strong> by {result["artist-credit"][0].name}
        </li>
      {/each}
    </ul>
  {/if}
</main>

<style>
  main {
    max-width: 600px;
    margin: 0 auto;
  }
</style>
