<script setup lang="ts">
import type { StatsNowPlayingEntry } from '~/types/api'

defineProps<{ entries: StatsNowPlayingEntry[] }>()
</script>

<template>
  <p v-if="entries.length === 0" class="text-base-content/60 text-sm">
    Nobody's listening right now.
  </p>
  <div v-else class="flex flex-col gap-2">
    <div v-for="(entry, i) in entries" :key="i" class="flex items-center gap-3">
      <div class="avatar">
        <div class="bg-base-300 flex w-8 items-center justify-center rounded-full">
          <img v-if="entry.user?.image_url" :src="entry.user.image_url" :alt="entry.user.username">
          <Icon v-else name="fa6-solid:user" size="14" />
        </div>
      </div>
      <p class="text-sm">
        <NuxtLink v-if="entry.user?.id" :to="`/users/${entry.user.id}`" class="font-medium hover:underline">
          {{ entry.user.display_name || entry.user.username }}
        </NuxtLink>
        <span v-else class="font-medium">Someone</span>
        is playing
        <NuxtLink :to="`/songs/${entry.song.id}`" class="hover:underline">{{ entry.song.name }}</NuxtLink>
      </p>
    </div>
  </div>
</template>
