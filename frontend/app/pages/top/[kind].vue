<script setup lang="ts">
import type { TopKind } from '~/types/api'

const validKinds: TopKind[] = ['artists', 'albums', 'tracks']

definePageMeta({
  validate: route => validKinds.includes(route.params.kind as TopKind),
})

const route = useRoute()
const router = useRouter()
const kind = computed(() => route.params.kind as TopKind)
const kindLabel = computed(() => ({ artists: 'Artists', albums: 'Albums', tracks: 'Tracks' } as const)[kind.value])

const auth = useAuthStore()
const scope = computed({
  get: () => (route.query.scope as string) ?? 'global',
  set: (v: string) => router.push({ query: { ...route.query, scope: v } }),
})

const timeframe = useTimeframe()
const timeframeQuery = computed(() => timeframe.toQuery())

useHead({ title: () => `Top ${kindLabel.value}` })
</script>

<template>
  <div>
    <div class="mb-6 flex flex-wrap items-center justify-between gap-3">
      <h1 class="font-display text-3xl font-semibold">
        Top {{ kindLabel }}
      </h1>
      <div v-if="auth.authed" class="join">
        <button
          class="btn btn-sm join-item"
          :class="scope === 'global' ? 'btn-primary' : 'btn-ghost'"
          @click="scope = 'global'"
        >
          Global
        </button>
        <button
          class="btn btn-sm join-item"
          :class="scope !== 'global' ? 'btn-primary' : 'btn-ghost'"
          @click="scope = String(auth.me!.id)"
        >
          Mine
        </button>
      </div>
    </div>

    <StatsPeriodSelector :timeframe="timeframe" />

    <div class="mt-6">
      <LeaderboardList :key="`${kind}-${scope}-${JSON.stringify(timeframeQuery)}`" :scope="scope" :kind="kind" :timeframe="timeframeQuery" :hero="false" />
    </div>
  </div>
</template>
