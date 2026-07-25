<script setup lang="ts">
const { loading: exporting, run: exportData } = useAsyncAction(async () => {
  const data = await useApi('/export')
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'canto-export.json'
  a.click()
  URL.revokeObjectURL(url)
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <p class="text-base-content/60 text-sm">
      Download your full listen history in Canto's own export format. This file can also be re-imported into any Canto instance.
    </p>
    <button class="btn btn-primary" :class="{ 'btn-disabled': exporting }" @click="exportData">
      <span v-if="exporting" class="loading loading-spinner loading-xs" /> Download export
    </button>
  </div>
</template>
