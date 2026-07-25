<script setup lang="ts">
import type { RewindEntry } from '~/types/api'

const props = defineProps<{
  title: string
  kind: 'artist' | 'album' | 'song'
  entries: RewindEntry[]
}>()

const top = computed(() => props.entries[0])
const rest = computed(() => props.entries.slice(1))
const href = computed(() => (top.value ? `/${props.kind}s/${top.value.id}` : ''))
const fallbackIcon = computed(() => ({ artist: 'fa6-solid:users', album: 'fa6-solid:compact-disc', song: 'fa6-solid:music' })[props.kind])

function listensLabel(entry: RewindEntry) {
  return `${entry.listen_count.toLocaleString()} listens`
}
function minutesLabel(entry: RewindEntry) {
  return `${Math.round(entry.minutes_listened).toLocaleString()} minutes`
}
// entrySubParts renders as "by Artist · Album Name", with artist/album individually linked when an id is known.
function entrySubParts(entry: RewindEntry) {
  const parts: { text: string, href?: string }[] = []
  if (entry.artist_name) parts.push({ text: `by ${entry.artist_name}`, href: entry.artist_id ? `/artists/${entry.artist_id}` : undefined })
  if (entry.album_name) parts.push({ text: entry.album_name, href: entry.album_id ? `/albums/${entry.album_id}` : undefined })
  return parts
}
</script>

<template>
  <div v-if="top" class="flex flex-col gap-4 sm:flex-row">
    <NuxtLink :to="href" class="group shrink-0">
      <div class="bg-base-300 h-[180px] w-[180px] overflow-hidden rounded-lg sm:h-[210px] sm:w-[210px]">
        <img v-if="top.image_url" :src="top.image_url" :alt="top.name" class="h-full w-full object-cover transition-transform group-hover:scale-105">
        <div v-else class="flex h-full w-full items-center justify-center">
          <Icon :name="fallbackIcon" size="40" class="text-base-content/30" />
        </div>
      </div>
    </NuxtLink>

    <div class="flex min-w-0 flex-col justify-center gap-1">
      <h4 class="text-base-content/60 text-xs font-semibold tracking-wide uppercase">
        {{ title }}
      </h4>
      <NuxtLink :to="href" class="hover:underline">
        <p class="font-display truncate text-2xl font-semibold sm:text-4xl">
          {{ top.name }}
        </p>
      </NuxtLink>
      <p class="text-base-content/60 text-sm">
        {{ listensLabel(top) }}
      </p>
      <p class="text-base-content/40 text-xs">
        {{ minutesLabel(top) }}
      </p>

      <div v-if="rest.length > 0" class="mt-2 flex flex-col gap-0.5">
        <MediaItemRow
          v-for="(e, i) in rest"
          :id="e.id"
          :key="e.id"
          :kind="kind"
          :name="e.name"
          :sub-parts="entrySubParts(e)"
          :image-url="e.image_url"
          :rank="i + 2"
          :trailing="listensLabel(e)"
          :trailing-sub="minutesLabel(e)"
          :blacklisted="e.blacklisted"
          :delta="e.delta"
        />
      </div>
    </div>
  </div>
</template>
