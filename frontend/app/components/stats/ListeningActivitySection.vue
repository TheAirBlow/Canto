<script setup lang="ts">
import type { ActivityResult, ClockCell } from '~/types/api'

const props = defineProps<{ scope: string }>()

const year = ref<number | null>(null)
const bounds = computed(() => yearBounds(year.value))
const periodQuery = computed(() => (year.value !== null
  ? { year: year.value }
  : { from: Math.floor(bounds.value.from.getTime() / 1000), to: Math.floor(bounds.value.to.getTime() / 1000) }))

const { data: activity, pending: activityPending } = useAsyncData(
  () => `dashboard-activity-${props.scope}-${year.value ?? ''}`,
  () => useApi<ActivityResult>(`/stats/${props.scope}/activity`, { query: { step: 'day', ...periodQuery.value }, ssr: false }),
  { lazy: true, server: false, watch: [year], default: (): ActivityResult => ({ buckets: [], longest_streak: 0, current_streak: 0 }) },
)
const activityShown = useDelayedPending(activityPending)

const { data: clock, pending: clockPending } = useAsyncData(
  () => `dashboard-clock-${props.scope}-${year.value ?? ''}`,
  () => useApi<ClockCell[]>(`/stats/${props.scope}/clock`, {
    query: { ...periodQuery.value, tz: Intl.DateTimeFormat().resolvedOptions().timeZone },
    ssr: false,
  }),
  { lazy: true, server: false, watch: [year], default: (): ClockCell[] => [] },
)
const clockShown = useDelayedPending(clockPending)
</script>

<template>
  <div class="bg-base-200 rounded-box border-base-300 border p-5">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <h3 class="text-base-content/60 text-sm font-semibold uppercase">
        Listening activity
      </h3>
      <YearPeriodSelector v-model:year="year" />
    </div>

    <div>
      <h4 class="text-base-content/50 mb-2 text-xs font-semibold uppercase">
        Activity
      </h4>
      <Transition name="fade" mode="out-in">
        <div v-if="activityShown" key="skeleton" class="skeleton h-[98px] w-full" />
        <div v-else key="content">
          <p v-if="activity.buckets.length === 0" class="text-base-content/60 text-sm">
            No listening activity yet.
          </p>
          <ActivityCalendar v-else :buckets="activity.buckets" :from="bounds.from" :to="bounds.to" />
        </div>
      </Transition>
      <div v-if="!activityShown" class="text-base-content/60 mt-3 flex gap-4 text-xs">
        <span>Current streak · <span class="text-primary font-semibold">{{ activity.current_streak }}d</span></span>
        <span>Longest streak · <span class="text-primary font-semibold">{{ activity.longest_streak }}d</span></span>
      </div>
    </div>

    <div class="mt-6">
      <h4 class="text-base-content/50 mb-2 text-xs font-semibold uppercase">
        Listening clock
      </h4>
      <Transition name="fade" mode="out-in">
        <div v-if="clockShown" key="skeleton" class="skeleton h-[102px] w-full" />
        <ClockHeatmap v-else key="content" :cells="clock" />
      </Transition>
    </div>
  </div>
</template>
