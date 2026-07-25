<script setup lang="ts">
import type { StatsNowPlayingEntry } from '~/types/api'

const props = defineProps<{ entries: StatsNowPlayingEntry[] }>()
const entry = computed(() => props.entries[0])
</script>

<template>
  <div v-if="entry" class="bg-base-200 rounded-box border-base-300 border flex items-center gap-4 p-4">
    <div class="avatar">
      <div class="bg-base-300 w-16 rounded-lg">
        <img v-if="entry.song.image_url" :src="entry.song.image_url" :alt="entry.song.name">
        <div v-else class="flex h-full w-full items-center justify-center">
          <Icon name="fa6-solid:music" size="24" />
        </div>
      </div>
    </div>
    <div>
      <NowPlayingBadge />
      <NuxtLink :to="`/songs/${entry.song.id}`" class="mt-1 block font-semibold hover:underline">
        {{ entry.song.name }}
      </NuxtLink>
    </div>
  </div>
</template>
