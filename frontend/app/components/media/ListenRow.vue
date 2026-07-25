<script setup lang="ts">
import { formatTimeAgo, type UseTimeAgoUnitNamesDefault } from '@vueuse/core'
import type { StatsListen } from '~/types/api'

const props = defineProps<{ listen: StatsListen, deletable?: boolean }>()
const emit = defineEmits<{ delete: [] }>()

// metaParts renders as "by Artist · Album Name · 5m 10s", with artist/album individually linked when an id is known.
const metaParts = computed(() => {
  const parts: { text: string, href?: string }[] = []
  if (props.listen.song.artist_name) {
    parts.push({ text: `by ${props.listen.song.artist_name}`, href: props.listen.song.artist_id ? `/artists/${props.listen.song.artist_id}` : undefined })
  }
  if (props.listen.song.album_name) {
    parts.push({ text: props.listen.song.album_name, href: props.listen.song.album_id ? `/albums/${props.listen.song.album_id}` : undefined })
  }
  const durationMs = props.listen.duration_played_ms ?? props.listen.song.duration_ms
  if (durationMs) parts.push({ text: formatDuration(durationMs) })
  return parts
})

function relativeTime(iso: string) {
  return formatTimeAgo<UseTimeAgoUnitNamesDefault>(new Date(iso), {
    messages: {
      justNow: 'just now',
      invalid: '',
      past: (n: string) => `${n} ago`,
      future: (n: string) => `in ${n}`,
      second: (n: number) => `${n}s`,
      minute: (n: number) => `${n}m`,
      hour: (n: number) => `${n}h`,
      day: (n: number) => `${n}d`,
      week: (n: number) => `${n}w`,
      month: (n: number) => `${n}mo`,
      year: (n: number) => `${n}y`,
    },
  })
}
</script>

<template>
  <div class="hover:bg-base-200 group relative flex items-center gap-3 rounded-lg p-2 transition-colors">
    <div class="avatar">
      <div class="bg-base-300 w-10 rounded">
        <img v-if="listen.song.image_url" :src="listen.song.image_url" :alt="listen.song.name">
        <div v-else class="flex h-full w-full items-center justify-center">
          <Icon name="fa6-solid:music" size="16" />
        </div>
      </div>
    </div>
    <div class="min-w-0 flex-1">
      <p class="truncate font-medium">
        <NuxtLink :to="`/songs/${listen.song.id}`" class="relative z-10 hover:underline">
          {{ listen.song.name }}
          <span class="absolute inset-0" />
        </NuxtLink>
      </p>
      <p v-if="metaParts.length" class="text-base-content/60 truncate text-xs">
        <template v-for="(part, i) in metaParts" :key="i">
          <span v-if="i > 0"> · </span>
          <NuxtLink v-if="part.href" :to="part.href" class="relative z-10 hover:underline">{{ part.text }}</NuxtLink>
          <span v-else>{{ part.text }}</span>
        </template>
      </p>
      <p v-if="listen.user" class="text-base-content/60 truncate text-xs">
        <NuxtLink v-if="listen.user.id" :to="`/users/${listen.user.id}`" class="relative z-10 hover:underline">
          {{ listen.user.display_name || listen.user.username }}
        </NuxtLink>
        <span v-else>Private user</span>
      </p>
    </div>
    <span class="text-base-content/50 shrink-0 text-xs">{{ relativeTime(listen.listened_at) }}</span>
    <button
      v-if="deletable"
      class="btn btn-ghost btn-xs btn-circle text-base-content/40 hover:text-error relative z-10 shrink-0 opacity-0 transition-opacity group-hover:opacity-100"
      aria-label="Delete listen"
      @click="emit('delete')"
    >
      <Icon name="fa6-solid:xmark" size="12" />
    </button>
  </div>
</template>
