<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { CurveType } from 'vue-chrts'
import type { TimeframeQuery } from '~/composables/useTimeframe'
import type { DiscoveryBucket } from '~/types/api'

const props = defineProps<{ scope: string, timeframe: TimeframeQuery }>()

const { discoveryStep: step } = storeToRefs(usePreferencesStore())
const stepOptions = ['day', 'week', 'month']

const { data: buckets, pending } = useAsyncData(
  `discovery-${props.scope}`,
  () => useApi<DiscoveryBucket[]>(`/stats/${props.scope}/discovery`, {
    query: { ...props.timeframe, step: step.value },
    ssr: false,
  }),
  { lazy: true, server: false, watch: [() => props.timeframe, step], default: (): DiscoveryBucket[] => [] },
)
const shown = useDelayedPending(pending)

const categories = {
  total: { name: 'Total listens', color: 'var(--color-base-content)' },
  discoveries: { name: 'New discoveries', color: 'var(--color-primary)' },
}

function xFormatter(_tick: number, i?: number) {
  const bucket = i !== undefined ? buckets.value[i] : undefined
  return bucket ? new Date(bucket.bucket).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : ''
}

function tooltipTitleFormatter(bucket: DiscoveryBucket) {
  const date = new Date(bucket.bucket)
  if (step.value === 'month') return date.toLocaleDateString(undefined, { month: 'long', year: 'numeric' })
  if (step.value === 'week') return `Week of ${date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}`
  return date.toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric', year: 'numeric' })
}
</script>

<template>
  <div>
    <div class="mb-2 flex items-center justify-end gap-1 text-xs">
      <span class="text-base-content/50">Step:</span>
      <button
        v-for="s in stepOptions"
        :key="s"
        class="rounded px-1.5 py-0.5 capitalize transition"
        :class="s === step ? 'bg-primary/20 text-primary font-medium' : 'text-base-content/60 hover:text-base-content'"
        @click="step = s"
      >
        {{ s }}
      </button>
    </div>

    <Transition name="fade" mode="out-in">
      <div v-if="shown" key="skeleton" class="skeleton h-[220px] w-full" />
      <p v-else-if="buckets.length === 0" key="empty" class="text-base-content/60 text-sm">
        Not enough data yet.
      </p>
      <AreaChart
        v-else
        key="content"
        :data="buckets"
        :categories="categories"
        :height="220"
        :x-formatter="xFormatter"
        :tooltip-title-formatter="tooltipTitleFormatter"
        :curve-type="CurveType.MonotoneX"
        :y-grid-line="true"
        :duration="0"
      />
    </Transition>
  </div>
</template>
