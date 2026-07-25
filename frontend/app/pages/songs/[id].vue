<script setup lang="ts">
import type { Alias, EntityListen, EntityListeningNow, EntitySummary, SongDetail } from '~/types/api'

const route = useRoute()
const router = useRouter()
const id = route.params.id as string

const scope = computed({
  get: () => (route.query.scope as string) ?? 'global',
  set: (v: string) => router.push({ query: { ...route.query, scope: v } }),
})

const song = await useApi<SongDetail>(`/songs/${id}`)
useHead({ title: song.name })

const { data: stats } = useAsyncData(`song-stats-${id}`, () => useApi<EntitySummary>(`/songs/${id}/stats`, { query: { scope: scope.value } }), { lazy: true, watch: [scope] })
const { data: aliases } = useAsyncData(`song-aliases-${id}`, () => useApi<Alias[]>(`/songs/${id}/aliases`), { lazy: true, default: () => [] })
const { data: nowPlaying } = useAsyncData(`song-now-playing-${id}`, () => useApi<EntityListeningNow[]>(`/songs/${id}/now-playing`, { query: { scope: scope.value } }), { lazy: true, default: () => [], watch: [scope] })

const { items: listens, done, loading, error, loadMore, reset: resetListens } = useInfiniteList<EntityListen>({
  kind: 'page',
  perPage: 20,
  fetchPage: async (page, perPage) => {
    const data = await useApi<{ listens: EntityListen[], total: number }>(`/songs/${id}/listens`, { query: { page, per_page: perPage, scope: scope.value } })
    return { items: data.listens, total: data.total }
  },
})
loadMore()
watch(scope, () => {
  resetListens()
  loadMore()
})
const listensShown = useDelayedPending(loading)
</script>

<template>
  <div>
    <EntityHeader :name="song.name" :image-url="song.image_url" :pinned="song.pinned" type-label="Song">
      <template #sub>
        <p class="text-base-content/70 text-sm">
          <template v-for="(artist, i) in song.artists" :key="artist.id">
            <NuxtLink :to="`/artists/${artist.id}`" class="hover:underline">{{ artist.name }}</NuxtLink><span v-if="i < song.artists.length - 1">, </span>
          </template>
          <template v-if="song.album">
            · <NuxtLink :to="`/albums/${song.album.id}`" class="hover:underline">{{ song.album.name }}</NuxtLink>
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
          <EntityAdminActions :id="song.id" kind="song" />
        </div>
      </template>
    </EntityHeader>

    <div class="mb-8 grid gap-6 lg:grid-cols-2">
      <div class="bg-base-200 rounded-box border-base-300 border p-4">
        <h2 class="text-base-content/60 mb-3 text-sm font-semibold uppercase">
          Interest over time
        </h2>
        <InterestGraph :scope="scope" entity-type="song" :entity-id="song.id" />
      </div>
      <div class="bg-base-200 rounded-box border-base-300 border p-4">
        <h2 class="text-base-content/60 mb-3 text-sm font-semibold uppercase">
          Activity
        </h2>
        <ActivityHeatmap :scope="scope" :song-id="song.id" />
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
