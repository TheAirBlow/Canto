<script setup lang="ts">
import type { RewindBundle } from '~/types/api'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { open } = useDrawer()

const monthNames = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']
const currentYear = new Date().getFullYear()
const currentMonth = new Date().getMonth() + 1

const scope = computed(() => (route.query.scope as string) ?? String(auth.me?.id ?? ''))
const isOwnRewind = computed(() => scope.value === String(auth.me?.id ?? ''))

const year = computed(() => Number(route.query.year) || currentYear)
const month = computed(() => (route.query.month ? Number(route.query.month) : undefined))
const yearly = computed(() => !month.value)

function isFutureMonth(y: number, m: number) {
  return y === currentYear && m > currentMonth
}

function setYear(y: number) {
  const query: Record<string, string> = { ...route.query as Record<string, string>, year: String(y) }
  delete query.month
  router.push({ query })
}

function setMonth(m: number) {
  if (isFutureMonth(year.value, m)) return
  router.push({ query: { ...route.query as Record<string, string>, year: String(year.value), month: String(m) } })
}

function setYearly() {
  const query: Record<string, string> = { ...route.query as Record<string, string> }
  delete query.month
  router.push({ query })
}

const title = computed(() => yearly.value ? `Your ${year.value} Rewind` : `Your ${monthNames[month.value! - 1]} ${year.value} Rewind`)
useHead({ title: () => title.value })

const { data: rewind, pending, error } = useAsyncData<RewindBundle>(
  () => `rewind-${scope.value}-${year.value}-${month.value ?? ''}`,
  () => useApi<RewindBundle>(`/stats/${scope.value}/rewind`, { query: { year: year.value, month: month.value }, ssr: false }),
  { lazy: true, server: false, watch: [scope, year, month] },
)
const shown = useDelayedPending(pending)

function formatMinutes(minutes: number) {
  const hours = Math.floor(minutes / 60)
  return hours < 1 ? `${Math.round(minutes)}m` : `${hours}h ${Math.round(minutes % 60)}m`
}

function formatTopDay(iso: string) {
  return new Date(iso).toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: 'numeric' })
}
</script>

<template>
  <div v-if="!auth.authed" class="flex flex-col items-center gap-3 py-24 text-center">
    <Icon name="fa6-solid:record-vinyl" size="32" class="text-base-content/40" />
    <p class="text-base-content/60">
      Log in to see your Rewind.
    </p>
    <button class="btn btn-primary btn-sm" @click="open('login')">
      Log in
    </button>
  </div>

  <div v-else class="mx-auto flex max-w-4xl flex-col items-center gap-8 lg:flex-row lg:items-start">
    <div class="flex w-full flex-col items-center gap-4 lg:w-56 lg:pt-4">
      <div class="join">
        <button class="btn btn-sm join-item" :class="yearly ? 'btn-primary' : 'btn-ghost'" @click="setYearly">
          Yearly
        </button>
        <button class="btn btn-sm join-item" :class="!yearly ? 'btn-primary' : 'btn-ghost'" @click="setMonth(month ?? currentMonth)">
          Monthly
        </button>
      </div>

      <div class="grid grid-cols-3 gap-1.5 lg:grid-cols-2">
        <button
          v-for="y in 6" :key="y"
          class="btn btn-sm"
          :class="year === currentYear - y + 1 ? 'btn-primary' : 'btn-ghost'"
          @click="setYear(currentYear - y + 1)"
        >
          {{ currentYear - y + 1 }}
        </button>
      </div>

      <div v-if="!yearly" class="grid grid-cols-3 gap-1.5 lg:grid-cols-2">
        <button
          v-for="(m, i) in monthNames" :key="m"
          class="btn btn-sm"
          :class="[month === i + 1 ? 'btn-primary' : 'btn-ghost', { 'btn-disabled': isFutureMonth(year, i + 1) }]"
          @click="setMonth(i + 1)"
        >
          {{ m.slice(0, 3) }}
        </button>
      </div>
    </div>

    <div class="bg-base-200 rounded-box border-base-300 w-full min-w-0 border p-6 sm:p-8">
      <h1 class="font-display mb-6 text-2xl font-semibold sm:text-3xl">
        {{ title }}<span v-if="!isOwnRewind">'s</span>
      </h1>

      <Transition name="fade" mode="out-in">
        <div v-if="shown" key="skeleton" class="flex flex-col gap-6">
          <div v-for="i in 3" :key="i" class="skeleton h-[180px] w-full" />
        </div>
        <p v-else-if="error" key="error" class="text-error text-sm">
          Couldn't load this Rewind.
        </p>
        <p v-else-if="rewind && rewind.top_artists.length === 0" key="empty" class="text-base-content/60 text-sm">
          Not enough listening data for this period yet.
        </p>
        <div v-else-if="rewind" key="content" class="flex flex-col gap-8">
          <RewindHero title="Top artist" kind="artist" :entries="rewind.top_artists" />
          <RewindHero title="Top album" kind="album" :entries="rewind.top_albums" />
          <RewindHero title="Top track" kind="song" :entries="rewind.top_tracks" />

          <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
            <StatTile label="Plays" :value="rewind.plays" />
            <StatTile label="Time listened" :value="formatMinutes(rewind.minutes_listened)" />
            <StatTile label="Plays / day" :value="rewind.avg_daily_plays.toFixed(1)" />
            <StatTile label="Artists" :value="rewind.unique_artists" />
            <StatTile label="Albums" :value="rewind.unique_albums" />
            <StatTile label="Tracks" :value="rewind.unique_tracks" />
            <StatTile label="New artists" :value="rewind.new_artists" />
            <StatTile label="New albums" :value="rewind.new_albums" />
            <StatTile label="New tracks" :value="rewind.new_tracks" />
          </div>

          <p v-if="rewind.top_day || rewind.longest_streak > 0" class="text-base-content/60 text-xs">
            <template v-if="rewind.top_day">
              Most active day · <span class="text-base-content font-medium">{{ formatTopDay(rewind.top_day) }}</span>
            </template>
            <template v-if="rewind.top_day && rewind.longest_streak > 0"> · </template>
            <template v-if="rewind.longest_streak > 0">
              Longest streak · <span class="text-base-content font-medium">{{ rewind.longest_streak }}d</span>
            </template>
          </p>
        </div>
      </Transition>
    </div>
  </div>
</template>
