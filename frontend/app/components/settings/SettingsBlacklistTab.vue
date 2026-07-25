<script setup lang="ts">
import { ApiError } from '~/composables/useApi'
import type { Artist } from '~/types/api'

const artists = ref<Artist[]>([])
const loading = ref(true)
const loadError = ref('')

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    artists.value = await useApi<Artist[]>('/artists/blacklist')
  } catch (err) {
    loadError.value = err instanceof ApiError ? (err.body?.detail ?? err.message) : 'Failed to load blacklist.'
  } finally {
    loading.value = false
  }
}
load()
const shown = useDelayedPending(loading)

const { run: remove } = useAsyncAction(async (id: number) => {
  await useApi(`/artists/${id}/blacklist`, { method: 'DELETE' })
  await load()
}, 'Removed from blacklist.')
</script>

<template>
  <div class="flex flex-col gap-4">
    <p class="text-base-content/60 text-xs">
      Blacklisted artists are excluded from your own stats.
    </p>
    <Transition name="fade" mode="out-in">
      <div v-if="shown" key="skeleton" class="skeleton h-20 w-full" />
      <p v-else-if="loadError" key="error" class="text-error text-sm">
        {{ loadError }} <button class="link" @click="load">Retry</button>
      </p>
      <p v-else-if="artists.length === 0" key="empty" class="text-base-content/60 text-sm">
        No blacklisted artists.
      </p>
      <div v-else key="content" class="flex flex-col gap-1">
        <div v-for="artist in artists" :key="artist.id" class="hover:bg-base-200 flex items-center justify-between rounded-lg p-2">
          <NuxtLink :to="`/artists/${artist.id}`" class="hover:underline">
            {{ artist.name }}
          </NuxtLink>
          <button class="btn btn-ghost btn-xs text-error" @click="remove(artist.id)">
            <Icon name="fa6-solid:trash" size="12" />
          </button>
        </div>
      </div>
    </Transition>
  </div>
</template>
