<script setup lang="ts">
const { loading: reindexing, run: reindex } = useAsyncAction(async () => {
  await useApi('/admin/reindex', { method: 'POST' })
}, 'Reindex triggered.')

const { loading: recomputing, run: recompute } = useAsyncAction(async () => {
  await useApi('/admin/stats/recompute', { method: 'POST' })
}, 'Recompute triggered.')
</script>

<template>
  <div class="flex flex-col gap-4">
    <div class="bg-base-200 rounded-lg p-3">
      <p class="font-medium">
        Rebuild search index
      </p>
      <p class="text-base-content/60 mt-1 text-xs">
        Runs in the background — check server logs for completion, there's no progress endpoint to poll.
      </p>
      <button class="btn btn-outline btn-sm mt-2" :class="{ 'btn-disabled': reindexing }" @click="reindex">
        <span v-if="reindexing" class="loading loading-spinner loading-xs" /> Trigger reindex
      </button>
    </div>
    <div class="bg-base-200 rounded-lg p-3">
      <p class="font-medium">
        Recompute stats
      </p>
      <p class="text-base-content/60 mt-1 text-xs">
        Rebuilds every cached stats rollup in the background.
      </p>
      <button class="btn btn-outline btn-sm mt-2" :class="{ 'btn-disabled': recomputing }" @click="recompute">
        <span v-if="recomputing" class="loading loading-spinner loading-xs" /> Trigger recompute
      </button>
    </div>
  </div>
</template>
