<script setup lang="ts">
import { ApiError } from '~/composables/useApi'
import type { SearchResult, Song } from '~/types/api'

// songSubParts renders as "by Artist · Album Name", with artist/album individually linked when an id is known.
function songSubParts(song: Song) {
  const parts: { text: string, href?: string }[] = []
  if (song.artist_name) parts.push({ text: `by ${song.artist_name}`, href: song.artist_id ? `/artists/${song.artist_id}` : undefined })
  if (song.album_name) parts.push({ text: song.album_name, href: song.album_id ? `/albums/${song.album_id}` : undefined })
  return parts
}

const { close } = useDrawer()
const query = ref('')
const debouncedQuery = refDebounced(query, 300)
const results = ref<SearchResult[]>([])
const loading = ref(false)
const error = ref('')

watch(debouncedQuery, async (q) => {
  if (!q.trim()) {
    results.value = []
    error.value = ''
    return
  }
  loading.value = true
  error.value = ''
  try {
    results.value = await useApi<SearchResult[]>('/search', { query: { q, type: 'artist,album,song,user', limit: 8 } })
  } catch (err) {
    results.value = []
    error.value = err instanceof ApiError ? (err.body?.detail ?? err.message) : 'Search is unavailable right now.'
  } finally {
    loading.value = false
  }
})
const shown = useDelayedPending(loading)

const groups = computed(() => [
  { label: 'Artists', results: results.value.filter(r => r.artist) },
  { label: 'Albums', results: results.value.filter(r => r.album) },
  { label: 'Tracks', results: results.value.filter(r => r.song) },
  { label: 'Users', results: results.value.filter(r => r.user) },
].filter(g => g.results.length > 0))

function resultKey(r: SearchResult, i: number) {
  return `${r.type}-${r.artist?.id ?? r.album?.id ?? r.song?.id ?? r.user?.id ?? i}`
}
</script>

<template>
  <AppModal max-width="max-w-lg" @close="close">
    <label class="input flex w-full items-center gap-2">
      <Icon name="fa6-solid:magnifying-glass" size="14" />
      <input v-model="query" type="text" class="grow" placeholder="Search artists, albums, tracks, users…" autofocus>
    </label>

    <div class="mt-4 max-h-96 overflow-y-auto">
      <Transition name="fade" mode="out-in">
        <div v-if="shown" key="loading" class="flex justify-center py-6">
          <span class="loading loading-spinner" />
        </div>
        <p v-else-if="error" key="error" class="text-error py-6 text-center text-sm">
          {{ error }}
        </p>
        <p v-else-if="query && results.length === 0" key="empty" class="text-base-content/60 py-6 text-center text-sm">
          No results.
        </p>
        <div v-else key="content" class="flex flex-col gap-2">
          <div v-for="(group, gi) in groups" :key="group.label" :class="{ 'border-base-300 border-t pt-2': gi > 0 }">
            <p class="text-base-content/50 mb-1 px-2 text-xs font-semibold uppercase">
              {{ group.label }}
            </p>
            <div class="flex flex-col gap-0.5">
              <template v-for="(r, i) in group.results" :key="resultKey(r, i)">
                <MediaItemRow v-if="r.artist" :id="r.artist.id" kind="artist" size="md" :name="r.artist.name" :image-url="r.artist.image_url" @click="close" />
                <MediaItemRow v-else-if="r.album" :id="r.album.id" kind="album" size="md" :name="r.album.name" :image-url="r.album.image_url" @click="close" />
                <MediaItemRow v-else-if="r.song" :id="r.song.id" kind="song" size="md" :name="r.song.name" :image-url="r.song.image_url" :sub-parts="songSubParts(r.song)" @click="close" />
                <NuxtLink v-else-if="r.user" :to="`/users/${r.user.id}`" class="hover:bg-base-200 flex items-center gap-3 rounded-lg p-2" @click="close">
                  <div class="avatar placeholder">
                    <div class="bg-neutral text-neutral-content w-12 rounded-full">
                      <img v-if="r.user.image_url" :src="r.user.image_url" :alt="r.user.username">
                      <Icon v-else name="fa6-solid:user" size="14" />
                    </div>
                  </div>
                  <span>{{ r.user.display_name || r.user.username }}</span>
                </NuxtLink>
              </template>
            </div>
          </div>
        </div>
      </Transition>
    </div>
  </AppModal>
</template>
