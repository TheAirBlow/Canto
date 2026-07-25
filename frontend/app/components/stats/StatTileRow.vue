<script setup lang="ts">
import type { StatsSummary } from '~/types/api'

const props = defineProps<{ summary: StatsSummary }>()

function formatCompactDuration(minutes: number) {
  const hours = Math.floor(minutes / 60)
  if (hours < 1) return `${Math.round(minutes)}m`
  const days = Math.floor(hours / 24)
  if (days < 1) return `${hours}h`
  return `${days}d ${hours % 24}h`
}

function formatSession(ms: number) {
  return formatCompactDuration(ms / 60000)
}

const stats = computed(() => [
  { label: 'Tracks', value: props.summary.unique_tracks, icon: 'fa6-solid:music' },
  { label: 'Albums', value: props.summary.unique_albums, icon: 'fa6-solid:compact-disc' },
  { label: 'Artists', value: props.summary.unique_artists, icon: 'fa6-solid:users' },
  { label: 'Plays per day', value: props.summary.avg_daily_plays.toFixed(1), icon: 'fa6-solid:chart-line' },
  { label: 'Tracks per artist', value: props.summary.tracks_per_artist.toFixed(1), icon: 'fa6-solid:list' },
  { label: 'Albums per artist', value: props.summary.albums_per_artist.toFixed(1), icon: 'fa6-solid:layer-group' },
  { label: 'Current streak', value: `${props.summary.current_streak}d`, icon: 'fa6-solid:fire' },
  { label: 'Longest streak', value: `${props.summary.longest_streak}d`, icon: 'fa6-solid:trophy' },
  { label: 'Avg session', value: formatSession(props.summary.avg_session_length_ms), icon: 'fa6-solid:stopwatch' },
])
</script>

<template>
  <div class="grid grid-flow-row-dense grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
    <div class="from-primary/15 to-primary/5 border-primary/20 rounded-box row-span-3 flex flex-col justify-center gap-4 border bg-gradient-to-br p-5">
      <div>
        <div class="text-base-content/60 flex items-center gap-2 text-xs font-medium tracking-wide uppercase">
          <Icon name="fa6-solid:clock" size="14" /> Time listened
        </div>
        <div class="flex items-baseline gap-1.5">
          <span class="font-display text-primary text-4xl font-semibold tracking-tight">{{ Math.round(summary.minutes_listened).toLocaleString() }}</span>
          <span class="text-base-content/60 text-sm">minutes</span>
        </div>
        <p class="text-base-content/50 text-xs">
          {{ formatCompactDuration(summary.minutes_listened) }}
        </p>
      </div>

      <div class="border-primary/20 border-t pt-4">
        <div class="text-base-content/60 flex items-center gap-2 text-xs font-medium tracking-wide uppercase">
          <Icon name="fa6-solid:play" size="14" /> Listens
        </div>
        <span class="font-display text-primary text-4xl font-semibold tracking-tight">{{ summary.listen_count.toLocaleString() }}</span>
      </div>

      <div class="border-primary/20 border-t pt-4">
        <div class="text-base-content/60 flex items-center gap-2 text-xs font-medium tracking-wide uppercase">
          <Icon name="fa6-solid:calendar-days" size="14" /> Days active
        </div>
        <span class="font-display text-primary text-4xl font-semibold tracking-tight">{{ summary.days_active.toLocaleString() }}</span>
      </div>
    </div>

    <StatTile v-for="stat in stats" :key="stat.label" :label="stat.label" :value="stat.value" :icon="stat.icon" />
  </div>
</template>
