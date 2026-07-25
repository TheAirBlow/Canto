<script setup lang="ts">
import type { Alias, Artist, ArtistDetail, EntityListen, EntityListeningNow, EntitySummary } from '~/types/api'

const route = useRoute()
const router = useRouter()
const id = route.params.id as string
const auth = useAuthStore()

const scope = computed({
  get: () => (route.query.scope as string) ?? 'global',
  set: (v: string) => router.push({ query: { ...route.query, scope: v } }),
})

const artist = await useApi<ArtistDetail>(`/artists/${id}`)
useHead({ title: artist.name })

const { data: stats } = useAsyncData(`artist-stats-${id}`, () => useApi<EntitySummary>(`/artists/${id}/stats`, { query: { scope: scope.value } }), { lazy: true, watch: [scope] })
const { data: aliases } = useAsyncData(`artist-aliases-${id}`, () => useApi<Alias[]>(`/artists/${id}/aliases`), { lazy: true, default: () => [] })
const { data: nowPlaying } = useAsyncData(`artist-now-playing-${id}`, () => useApi<EntityListeningNow[]>(`/artists/${id}/now-playing`, { query: { scope: scope.value } }), { lazy: true, default: () => [], watch: [scope] })

const { data: blacklist } = useAsyncData(`artist-blacklist-${id}`, () => auth.authed ? useApi<Artist[]>('/artists/blacklist') : Promise.resolve([]), { lazy: true, default: () => [] })
const blacklisted = computed(() => blacklist.value.some(a => String(a.id) === id))

const toast = useToast()
const { loading: blacklistBusy, run: toggleBlacklist } = useAsyncAction(async () => {
  const wasBlacklisted = blacklisted.value
  await useApi(`/artists/${id}/blacklist`, { method: wasBlacklisted ? 'DELETE' : 'PUT' })
  if (wasBlacklisted) blacklist.value = blacklist.value.filter(a => String(a.id) !== id)
  else blacklist.value = [...blacklist.value, artist]
  toast.success(wasBlacklisted ? 'Removed from your blacklist.' : 'Blacklisted for you.')
})

const { items: listens, done, loading, error, loadMore, reset: resetListens } = useInfiniteList<EntityListen>({
  kind: 'page',
  perPage: 20,
  fetchPage: async (page, perPage) => {
    const data = await useApi<{ listens: EntityListen[], total: number }>(`/artists/${id}/listens`, { query: { page, per_page: perPage, scope: scope.value } })
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
    <EntityHeader :name="artist.name" :image-url="artist.image_url" :description="artist.description" :pinned="artist.pinned" type-label="Artist">
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
          <div class="flex gap-2">
            <button
              v-if="auth.authed"
              class="btn btn-outline btn-sm"
              :class="{ 'btn-disabled': blacklistBusy }"
              @click="toggleBlacklist"
            >
              <Icon name="fa6-solid:ban" size="12" /> {{ blacklisted ? 'Unblacklist' : 'Blacklist for me' }}
            </button>
            <EntityAdminActions :id="artist.id" kind="artist" />
          </div>
        </div>
      </template>
    </EntityHeader>

    <div v-if="artist.albums.length > 0" class="mb-8">
      <MediaShelf title="Albums" :items="artist.albums.map(a => ({ id: a.id, name: a.name, imageUrl: a.image_url }))" kind="album" />
    </div>

    <div v-if="artist.songs.length > 0" class="mb-8">
      <MediaShelf title="Songs" :items="artist.songs.map(s => ({ id: s.id, name: s.name, imageUrl: s.image_url }))" kind="song" />
    </div>

    <div class="mb-8 grid gap-6 lg:grid-cols-2">
      <div class="bg-base-200 rounded-box border-base-300 border p-4">
        <h2 class="text-base-content/60 mb-3 text-sm font-semibold uppercase">
          Interest over time
        </h2>
        <InterestGraph :scope="scope" entity-type="artist" :entity-id="artist.id" />
      </div>
      <div class="bg-base-200 rounded-box border-base-300 border p-4">
        <h2 class="text-base-content/60 mb-3 text-sm font-semibold uppercase">
          Activity
        </h2>
        <ActivityHeatmap :scope="scope" :artist-id="artist.id" />
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
