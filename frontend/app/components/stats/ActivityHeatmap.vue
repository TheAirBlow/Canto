<script setup lang="ts">
import type { ActivityResult } from '~/types/api'

const props = defineProps<{ scope: string, artistId?: number, albumId?: number, songId?: number }>()

const year = ref<number | null>(null)
const bounds = computed(() => yearBounds(year.value))

const { data, pending } = useAsyncData(
  () => `activity-${props.scope}-${props.artistId ?? ''}-${props.albumId ?? ''}-${props.songId ?? ''}-${year.value ?? ''}`,
  () => {
    const { from, to } = bounds.value
    return useApi<ActivityResult>(`/stats/${props.scope}/activity`, {
      query: {
        step: 'day',
        ...(year.value !== null ? { year: year.value } : { from: Math.floor(from.getTime() / 1000), to: Math.floor(to.getTime() / 1000) }),
        artist_id: props.artistId,
        album_id: props.albumId,
        song_id: props.songId,
      },
      ssr: false,
    })
  },
  { lazy: true, server: false, watch: [year, () => props.scope], default: (): ActivityResult => ({ buckets: [], longest_streak: 0, current_streak: 0 }) },
)
const shown = useDelayedPending(pending)
</script>

<template>
  <div>
    <div class="mb-3 flex justify-end">
      <YearPeriodSelector v-model:year="year" />
    </div>

    <Transition name="fade" mode="out-in">
      <div v-if="shown" key="skeleton" class="skeleton h-[98px] w-full" />
      <div v-else key="content">
        <p v-if="data.buckets.length === 0" class="text-base-content/60 text-sm">
          No listening activity yet.
        </p>
        <ActivityCalendar v-else :buckets="data.buckets" :from="bounds.from" :to="bounds.to" />
      </div>
    </Transition>

    <div v-if="!shown" class="text-base-content/60 mt-3 flex gap-4 text-xs">
      <span>Current streak · <span class="text-primary font-semibold">{{ data.current_streak }}d</span></span>
      <span>Longest streak · <span class="text-primary font-semibold">{{ data.longest_streak }}d</span></span>
    </div>
  </div>
</template>
