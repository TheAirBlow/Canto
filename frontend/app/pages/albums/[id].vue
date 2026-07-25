<script setup lang="ts">
import type { Alias, AlbumDetail, EntityListen, EntityListeningNow, EntitySummary } from '~/types/api'

const route = useRoute()
const router = useRouter()
const id = route.params.id as string

const scope = computed({
  get: () => (route.query.scope as string) ?? 'global',
  set: (v: string) => router.push({ query: { ...route.query, scope: v } }),
})

const album = await useApi<AlbumDetail>(`/albums/${id}`)
useHead({ title: album.name })

const { data: stats } = useAsyncData(`album-stats-${id}`, () => useApi<EntitySummary>(`/albums/${id}/stats`, { query: { scope: scope.value } }), { lazy: true, watch: [scope] })
const { data: aliases } = useAsyncData(`album-aliases-${id}`, () => useApi<Alias[]>(`/albums/${id}/aliases`), { lazy: true, default: () => [] })
const { data: nowPlaying } = useAsyncData(`album-now-playing-${id}`, () => useApi<EntityListeningNow[]>(`/albums/${id}/now-playing`, { query: { scope: scope.value } }), { lazy: true, default: () => [], watch: [scope] })

const { items: listens, done, loading, error, loadMore, reset: resetListens } = useInfiniteList<EntityListen>({
  kind: 'page',
  perPage: 20,
  fetchPage: async (page, perPage) => {
    const data = await useApi<{ listens: EntityListen[], total: number }>(`/albums/${id}/listens`, { query: { page, per_page: perPage, scope: scope.value } })
    return { items: data.listens, total: data.total }
  },
})
loadMore()
watch(scope, () => {
  resetListens()
  loadMore()
})
const listensShown = useDelayedPending(loading)

const trackCap = 20
const tracksExpanded = ref(false)
const visibleTracks = computed(() => tracksExpanded.value ? album.tracks : album.tracks.slice(0, trackCap))
</script>

<template>
  <div>
    <EntityHeader :name="album.name" :image-url="album.image_url" :description="album.description" :pinned="album.pinned" type-label="Album">
      <template #sub>
        <p v-if="album.artists.length > 0" class="text-base-content/70 text-sm">
          By
          <template v-for="(artist, i) in album.artists" :key="artist.id">
            <NuxtLink :to="`/artists/${artist.id}`" class="hover:underline">{{ artist.name }}</NuxtLink><span v-if="i < album.artists.length - 1">, </span>
          </template>
        </p>
      </template>
      <template #meta>
        <div class="flex flex-col gap-3">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <EntityStatsSummary :stats="stats" />
            <EntityScopeSwitch v-model:scope="scope" />
          </div>
          <EntityNowPlayingList :entries="nowPlaying" />
          <div v-if="aliases.length > 0" class="flex flex-wrap gap-1.5">
            <span v-for="alias in aliases" :key="alias.id" class="badge badge-ghost badge-sm">{{ alias.alias }}</span>
          </div>
          <EntityAdminActions :id="album.id" kind="album" />
        </div>
      </template>
    </EntityHeader>

    <div v-if="album.tracks.length > 0" class="mb-8">
      <h2 class="mb-3 text-lg font-bold">
        Tracklist
      </h2>
      <div class="flex flex-col gap-1">
        <MediaItemRow
          v-for="track in visibleTracks"
          :id="track.id"
          :key="track.id"
          kind="song"
          :name="track.name"
          :image-url="track.image_url"
          :rank="track.track_number"
        />
      </div>
      <button v-if="!tracksExpanded && album.tracks.length > trackCap" class="btn btn-ghost btn-sm mt-2" @click="tracksExpanded = true">
        Show all {{ album.tracks.length }} tracks
      </button>
    </div>

    <div class="mb-8 grid gap-6 lg:grid-cols-2">
      <div class="bg-base-200 rounded-box border-base-300 border p-4">
        <h2 class="text-base-content/60 mb-3 text-sm font-semibold uppercase">
          Interest over time
        </h2>
        <InterestGraph :scope="scope" entity-type="album" :entity-id="album.id" />
      </div>
      <div class="bg-base-200 rounded-box border-base-300 border p-4">
        <h2 class="text-base-content/60 mb-3 text-sm font-semibold uppercase">
          Activity
        </h2>
        <ActivityHeatmap :scope="scope" :album-id="album.id" />
      </div>
    </div>

    <div>
      <h2 class="mb-3 text-lg font-bold">
        Recent listens
      </h2>
      <Transition name="fade" mode="out-in">
        <div v-if="listens.length === 0 && listensShown" key="skeleton" class="flex flex-col gap-2">
          <div v-for="i in 5" :key="i" class="skeleton h-10 w-full" />
        </div>
        <p v-else-if="error" key="error" class="text-error text-sm">
          Failed to load listens.
        </p>
        <p v-else-if="listens.length === 0" key="empty" class="text-base-content/60 text-sm">
          No listens yet.
        </p>
        <div v-else key="content" class="flex flex-col gap-1">
          <EntityListenRow v-for="(listen, i) in listens" :key="i" :listen="listen" />
          <InfiniteSentinel :disabled="done || loading" @load="loadMore" />
        </div>
      </Transition>
    </div>
  </div>
</template>
