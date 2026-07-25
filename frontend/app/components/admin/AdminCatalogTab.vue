<script setup lang="ts">
import { ApiError } from '~/composables/useApi'
import type { Album, Artist, SearchResult, Song } from '~/types/api'

type EntityKind = 'artist' | 'album' | 'song'
type BrowseEntity = Artist | Album | Song

const kinds: { value: EntityKind, label: string, path: string, searchType: string }[] = [
  { value: 'artist', label: 'Artists', path: 'artists', searchType: 'artist' },
  { value: 'album', label: 'Albums', path: 'albums', searchType: 'album' },
  { value: 'song', label: 'Tracks', path: 'songs', searchType: 'song' },
]
const kind = ref<EntityKind>('artist')
const activeKind = computed(() => kinds.find(k => k.value === kind.value)!)

const query = ref('')
const debouncedQuery = refDebounced(query, 300)
const searching = computed(() => debouncedQuery.value.trim().length > 0)

const searchResults = ref<SearchResult[]>([])
const searchError = ref('')
watch([debouncedQuery, kind], async ([q]) => {
  if (!q.trim()) {
    searchResults.value = []
    searchError.value = ''
    return
  }
  try {
    searchResults.value = await useApi<SearchResult[]>('/search', { query: { q, type: activeKind.value.searchType, limit: 20 } })
    searchError.value = ''
  } catch (err) {
    searchResults.value = []
    searchError.value = err instanceof ApiError ? (err.body?.detail ?? err.message) : 'Search is unavailable right now.'
  }
})

const { items, done, loading, error, loadMore, reset } = useInfiniteList<BrowseEntity>({
  kind: 'cursor',
  limit: 30,
  fetchPage: async (after, limit) => {
    const rows = await useApi<BrowseEntity[]>(`/${activeKind.value.path}`, { query: { after, limit } })
    return { items: rows, nextAfter: rows.length > 0 ? rows[rows.length - 1]!.id : undefined }
  },
})
loadMore()
const shown = useDelayedPending(loading)

watch(kind, () => {
  reset()
  loadMore()
})

const selected = ref<{ kind: EntityKind, id: number, name: string } | null>(null)
function selectEntity(k: EntityKind, id: number, name: string) {
  selected.value = { kind: k, id, name }
}
function selectSearchResult(r: SearchResult) {
  if (r.artist) selectEntity('artist', r.artist.id, r.artist.name)
  else if (r.album) selectEntity('album', r.album.id, r.album.name)
  else if (r.song) selectEntity('song', r.song.id, r.song.name)
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <div class="flex gap-2">
      <button
        v-for="k in kinds"
        :key="k.value"
        class="btn btn-sm"
        :class="kind === k.value ? 'btn-primary' : 'btn-ghost'"
        @click="kind = k.value"
      >
        {{ k.label }}
      </button>
    </div>

    <input v-model="query" type="text" :placeholder="`Search ${activeKind.label.toLowerCase()}…`" class="input input-sm w-full">

    <Transition name="fade" mode="out-in">
      <div v-if="searching" key="search" class="flex flex-col gap-1">
        <p v-if="searchError" class="text-error text-xs">
          {{ searchError }}
        </p>
        <p v-else-if="searchResults.length === 0" class="text-base-content/60 text-sm">
          No matches.
        </p>
        <MediaItemRow
          v-for="(r, i) in searchResults"
          :id="(r.artist ?? r.album ?? r.song)!.id"
          :key="i"
          :kind="kind"
          :name="(r.artist ?? r.album ?? r.song)!.name"
          :image-url="(r.artist ?? r.album ?? r.song)!.image_url"
          :linkable="false"
          @click.prevent="selectSearchResult(r)"
        />
      </div>

      <div v-else key="browse" class="flex flex-col gap-1">
        <div v-if="items.length === 0 && shown" class="flex flex-col gap-2">
          <div v-for="i in 6" :key="i" class="skeleton h-12 w-full" />
        </div>
        <p v-else-if="error" class="text-error text-sm">
          Failed to load.
        </p>
        <p v-else-if="items.length === 0" class="text-base-content/60 text-sm">
          Nothing in the catalog yet.
        </p>
        <template v-else>
          <MediaItemRow
            v-for="entity in items"
            :id="entity.id"
            :key="entity.id"
            :kind="kind"
            :name="entity.name"
            :image-url="entity.image_url"
            :linkable="false"
            @click.prevent="selectEntity(kind, entity.id, entity.name)"
          />
          <InfiniteSentinel :disabled="done || loading" @load="loadMore" />
        </template>
      </div>
    </Transition>

    <template v-if="selected">
      <div class="divider">
        {{ selected.name }}
      </div>
      <EntityEditDrawer :id="selected.id" :key="`${selected.kind}-${selected.id}`" :kind="selected.kind" />
    </template>
  </div>
</template>
