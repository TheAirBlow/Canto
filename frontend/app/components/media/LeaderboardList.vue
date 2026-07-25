<script setup lang="ts">
import type { TopEntry, TopKind } from '~/types/api'
import type { TimeframeQuery } from '~/composables/useTimeframe'

const props = withDefaults(
  defineProps<{ scope: string, kind: TopKind, timeframe?: TimeframeQuery, limit?: number, hero?: boolean, infinite?: boolean }>(),
  { timeframe: undefined, limit: undefined, hero: true, infinite: true },
)

const singularKind = computed(() => ({ artists: 'artist', albums: 'album', tracks: 'song' } as const)[props.kind])

const sortBy = ref<'listens' | 'minutes'>('listens')

function entrySubParts(entry: TopEntry) {
  const parts: { text: string, href?: string }[] = []
  if (entry.artist_name) parts.push({ text: `by ${entry.artist_name}`, href: entry.artist_id ? `/artists/${entry.artist_id}` : undefined })
  if (entry.album_name) parts.push({ text: entry.album_name, href: entry.album_id ? `/albums/${entry.album_id}` : undefined })
  return parts
}

function listensLabel(entry: TopEntry) {
  return `${entry.listen_count.toLocaleString()} listens`
}
function minutesLabel(entry: TopEntry) {
  return `${Math.round(entry.minutes_listened).toLocaleString()} minutes`
}
const primaryLabel = computed(() => (sortBy.value === 'minutes' ? minutesLabel : listensLabel))
const secondaryLabel = computed(() => (sortBy.value === 'minutes' ? listensLabel : minutesLabel))

const { items, done, loading, error, loadMore, reset } = useInfiniteList<TopEntry>({
  kind: 'page',
  perPage: props.limit ?? 10,
  fetchPage: async (page, perPage) => {
    const data = await useApi<TopEntry[]>(`/stats/${props.scope}/top/${props.kind}`, {
      query: { page, per_page: perPage, sort: sortBy.value, ...props.timeframe },
      ssr: false,
    })
    return { items: data }
  },
})

loadMore()
watch(sortBy, () => {
  reset()
  loadMore()
})
const shown = useDelayedPending(loading)
</script>

<template>
  <div>
    <div class="join mb-2 flex justify-end">
      <button type="button" class="btn btn-xs join-item" :class="sortBy === 'listens' ? 'btn-primary' : 'btn-ghost'" @click="sortBy = 'listens'">
        Listens
      </button>
      <button type="button" class="btn btn-xs join-item" :class="sortBy === 'minutes' ? 'btn-primary' : 'btn-ghost'" @click="sortBy = 'minutes'">
        Minutes
      </button>
    </div>
    <Transition name="fade" mode="out-in">
      <div v-if="items.length === 0 && shown" key="skeleton" class="flex flex-col gap-2">
        <div v-for="i in 5" :key="i" class="skeleton h-12 w-full" />
      </div>
      <p v-else-if="error" key="error" class="text-error text-sm">
        Failed to load.
      </p>
      <p v-else-if="items.length === 0" key="empty" class="text-base-content/60 text-sm">
        Nothing here yet.
      </p>
      <div v-else key="content" class="flex flex-col gap-1">
        <div
          v-if="hero"
          class="group rounded-box relative mb-2 aspect-[21/9] overflow-hidden sm:aspect-[6/3]"
          :class="{ 'opacity-50': items[0]!.blacklisted }"
        >
          <img
            v-if="items[0]!.image_url"
            :src="items[0]!.image_url"
            :alt="items[0]!.name"
            class="h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
          >
          <div v-else class="bg-base-300 flex h-full w-full items-center justify-center">
            <Icon :name="singularKind === 'artist' ? 'fa6-solid:users' : singularKind === 'album' ? 'fa6-solid:compact-disc' : 'fa6-solid:music'" size="40" class="text-base-content/30" />
          </div>
          <div class="from-base-100 via-base-100/50 absolute inset-0 bg-gradient-to-t to-transparent" />
          <div class="absolute inset-x-0 bottom-0 p-4 sm:p-5">
            <p v-if="entrySubParts(items[0]!).length" class="text-base-content/70 text-xs">
              <template v-for="(part, i) in entrySubParts(items[0]!)" :key="i">
                <span v-if="i > 0"> · </span>
                <NuxtLink v-if="part.href" :to="part.href" class="relative z-10 hover:underline">{{ part.text }}</NuxtLink>
                <span v-else>{{ part.text }}</span>
              </template>
            </p>
            <h3 class="font-display truncate text-2xl font-semibold sm:text-3xl">
              <NuxtLink :to="`/${singularKind}s/${items[0]!.id}`" class="relative z-10 hover:underline">
                {{ items[0]!.name }}
                <span class="absolute inset-0" />
              </NuxtLink>
            </h3>
            <div class="flex items-end gap-2">
              <div class="text-base-content/60 text-sm">
                <p>{{ primaryLabel(items[0]!) }}</p>
                <p class="text-base-content/40 text-xs">
                  {{ secondaryLabel(items[0]!) }}
                </p>
              </div>
              <span v-if="items[0]!.blacklisted" class="badge badge-ghost badge-xs gap-1 normal-case">
                <Icon name="fa6-solid:ban" size="8" /> Blacklisted
              </span>
            </div>
          </div>
        </div>

        <MediaItemRow
          v-for="(entry, i) in hero ? items.slice(1) : items"
          :id="entry.id"
          :key="entry.id"
          :kind="singularKind"
          :name="entry.name"
          :image-url="entry.image_url"
          :sub-parts="entrySubParts(entry)"
          :rank="hero ? i + 2 : i + 1"
          :trailing="primaryLabel(entry)"
          :trailing-sub="secondaryLabel(entry)"
          :blacklisted="entry.blacklisted"
        />
        <InfiniteSentinel v-if="infinite" :disabled="done || loading" @load="loadMore" />
      </div>
    </Transition>
  </div>
</template>
