<script setup lang="ts">
import type { EntitySummary } from '~/types/api'

defineProps<{ stats?: EntitySummary }>()
</script>

<template>
  <div v-if="stats" class="flex flex-wrap items-center gap-x-6 gap-y-2">
    <span class="text-primary flex items-baseline gap-1.5">
      <span class="font-display text-2xl font-semibold sm:text-3xl">{{ stats.plays.toLocaleString() }}</span>
      <span class="text-base-content/60 text-sm">plays</span>
    </span>
    <span v-if="stats.unique_listeners != null" class="text-base-content/60 flex items-center gap-1.5 text-sm">
      <Icon name="fa6-solid:users" size="12" /> {{ stats.unique_listeners }} listeners
    </span>
    <span v-if="stats.first_listened_at" class="text-base-content/60 flex items-center gap-1.5 text-sm">
      <Icon name="fa6-solid:calendar-days" size="12" /> Since {{ new Date(stats.first_listened_at).toLocaleDateString() }}
    </span>
  </div>
</template>
