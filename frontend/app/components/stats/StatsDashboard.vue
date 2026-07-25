<script setup lang="ts">
import type { SourceEntry, StatsListen, StatsNowPlayingEntry, StatsSummary, TopKind } from '~/types/api'

const props = defineProps<{ scope: string }>()

const timeframe = useTimeframe()
const timeframeQuery = computed(() => timeframe.toQuery())
const timeframeKey = computed(() => JSON.stringify(timeframeQuery.value))

const { data: summary, pending: summaryPending } = useAsyncData(
  `stats-summary-${props.scope}`,
  () => useApi<StatsSummary>(`/stats/${props.scope}/summary`, { query: { ...timeframeQuery.value }, ssr: false }),
  { lazy: true, server: false, watch: [timeframeQuery] },
)
const summaryShown = useDelayedPending(summaryPending)
const { data: sources, pending: sourcesPending } = useAsyncData(
  `stats-sources-${props.scope}`,
  () => useApi<SourceEntry[]>(`/stats/${props.scope}/sources`, { query: { ...timeframeQuery.value }, ssr: false }),
  { lazy: true, server: false, watch: [timeframeQuery], default: (): SourceEntry[] => [] },
)
const sourcesShown = useDelayedPending(sourcesPending)
const { data: nowPlaying } = useAsyncData(
  `stats-now-playing-${props.scope}`,
  () => useApi<StatsNowPlayingEntry[]>(`/stats/${props.scope}/now-playing`, { ssr: false }),
  { lazy: true, server: false, default: (): StatsNowPlayingEntry[] => [] },
)

const {
  items: listens,
  done: listensDone,
  loading: listensLoading,
  error: listensError,
  loadMore: loadMoreListens,
  reset: resetListens,
} = useInfiniteList<StatsListen>({
  kind: 'page',
  perPage: 20,
  fetchPage: async (page, perPage) => {
    const data = await useApi<{ listens: StatsListen[], total: number }>(`/stats/${props.scope}/listens`, {
      query: { page, per_page: perPage, ...timeframeQuery.value },
      ssr: false,
    })
    return { items: data.listens, total: data.total }
  },
})
loadMoreListens()
watch(timeframeQuery, () => {
  resetListens()
  loadMoreListens()
})
const listensShown = useDelayedPending(listensLoading)

const auth = useAuthStore()
function isOwnListen(listen: StatsListen) {
  return props.scope === 'global' ? listen.user?.id === auth.me?.id : props.scope === String(auth.me?.id)
}

const { run: deleteListen } = useAsyncAction(async (listen: StatsListen) => {
  await useApi(`/listens/${listen.id}`, { method: 'DELETE' })
  listens.value = listens.value.filter(l => l.id !== listen.id)
})

const topKinds: TopKind[] = ['artists', 'albums', 'tracks']
</script>

<template>
  <div class="flex flex-col gap-6">
    <NowPlayingList v-if="scope === 'global'" :entries="nowPlaying" />
    <NowPlayingBanner v-else :entries="nowPlaying" />

    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="text-base-content/60 text-sm font-semibold uppercase">
        Overview
      </h2>
      <StatsPeriodSelector :timeframe="timeframe" />
    </div>

    <Transition name="fade" mode="out-in">
      <div v-if="summaryShown" key="skeleton" class="skeleton h-40 w-full sm:h-32" />
      <StatTileRow v-else-if="summary" key="content" :summary="summary" />
    </Transition>

    <div class="grid gap-6 lg:grid-cols-4">
      <div class="lg:col-span-3 bg-base-200 rounded-box border-base-300 border p-5">
        <h3 class="text-base-content/60 mb-3 text-sm font-semibold uppercase">
          Discovery
        </h3>
        <DiscoveryGraph :scope="scope" :timeframe="timeframeQuery" />
      </div>
      <div class="lg:col-span-1 bg-base-200 rounded-box border-base-300 border p-5">
        <h3 class="text-base-content/60 mb-3 text-sm font-semibold uppercase">
          Sources
        </h3>
        <Transition name="fade" mode="out-in">
          <div v-if="sourcesShown" key="skeleton" class="skeleton h-40 w-full" />
          <SourcesBreakdown v-else key="content" :sources="sources" />
        </Transition>
      </div>
    </div>

    <div class="grid gap-6 lg:grid-cols-3">
      <div v-for="k in topKinds" :key="k" class="bg-base-200 rounded-box border-base-300 border p-5">
        <div class="mb-3 flex items-center justify-between gap-2">
          <h3 class="text-base-content/60 text-sm font-semibold uppercase">
            Top {{ k }}
          </h3>
          <NuxtLink :to="`/top/${k}?scope=${scope}`" class="text-base-content/60 hover:text-base-content text-xs font-medium">
            Full list →
          </NuxtLink>
        </div>
        <LeaderboardList
          :key="`${k}-${timeframeKey}`"
          :scope="scope"
          :kind="k"
          :timeframe="timeframeQuery"
          :limit="10"
          :infinite="false"
        />
      </div>
    </div>

    <ListeningActivitySection :scope="scope" />

    <div class="bg-base-200 rounded-box border-base-300 border p-5">
      <h3 class="text-base-content/60 mb-3 text-sm font-semibold uppercase">
        Recent listens
      </h3>
      <Transition name="fade" mode="out-in">
        <div v-if="listens.length === 0 && listensShown" key="skeleton" class="flex flex-col gap-2">
          <div v-for="i in 5" :key="i" class="skeleton h-14 w-full" />
        </div>
        <p v-else-if="listensError" key="error" class="text-error text-sm">
          Failed to load listens.
        </p>
        <p v-else-if="listens.length === 0" key="empty" class="text-base-content/60 text-sm">
          No listens yet.
        </p>
        <div v-else key="content" class="flex flex-col gap-1">
          <ListenRow
            v-for="listen in listens"
            :key="listen.id"
            :listen="listen"
            :deletable="isOwnListen(listen)"
            @delete="deleteListen(listen)"
          />
          <InfiniteSentinel :disabled="listensDone || listensLoading" @load="loadMoreListens" />
        </div>
      </Transition>
    </div>
  </div>
</template>
