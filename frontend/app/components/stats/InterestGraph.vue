<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { CurveType } from 'vue-chrts'
import type { InterestBucket } from '~/types/api'

const props = defineProps<{ scope: string, entityType: 'artist' | 'album' | 'song', entityId: number }>()

const { interestStep: step } = storeToRefs(usePreferencesStore())
const stepOptions = ['day', 'week', 'month']

const { data: buckets, pending } = useAsyncData(
  `interest-${props.entityType}-${props.entityId}-${props.scope}`,
  () => useApi<InterestBucket[]>(`/stats/${props.scope}/interest/${props.entityType}/${props.entityId}`, {
    query: { step: step.value },
    ssr: false,
  }),
  { lazy: true, server: false, watch: [() => props.scope, step], default: (): InterestBucket[] => [] },
)
const shown = useDelayedPending(pending)

const categories = { listen_count: { name: 'Listens', color: 'var(--color-primary)' } }

function xFormatter(_tick: number, i?: number) {
  const bucket = i !== undefined ? buckets.value[i] : undefined
  return bucket ? new Date(bucket.bucket).toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) : ''
}

function tooltipTitleFormatter(bucket: InterestBucket) {
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
      <div v-if="shown" key="skeleton" class="skeleton h-[200px] w-full" />
      <p v-else-if="buckets.length === 0" key="empty" class="text-base-content/60 text-sm">
        Not enough data yet.
      </p>
      <AreaChart
        v-else
        key="content"
        :data="buckets"
        :categories="categories"
        :height="200"
        :x-formatter="xFormatter"
        :tooltip-title-formatter="tooltipTitleFormatter"
        :curve-type="CurveType.MonotoneX"
        :duration="0"
      />
    </Transition>
  </div>
</template>
