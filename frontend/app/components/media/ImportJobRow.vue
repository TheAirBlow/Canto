<script setup lang="ts">
import type { ImportJob } from '~/types/api'

const props = defineProps<{ job: ImportJob }>()

const statusColor: Record<string, string> = {
  queued: 'badge-ghost',
  running: 'badge-info',
  paused: 'badge-warning',
  completed: 'badge-success',
  failed: 'badge-error',
  cancelled: 'badge-ghost',
}

const { data } = usePolling<ImportJob>(
  () => useApi<ImportJob>(`/import/${props.job.id}`),
  j => ['completed', 'failed', 'cancelled'].includes(j.status),
)
const job = computed(() => data.value ?? props.job)
const service = computed(() => serviceMeta(job.value.service))
const cancellable = computed(() => ['queued', 'running', 'paused'].includes(job.value.status))
const progress = computed(() => (job.value.total_items > 0 ? Math.round((job.value.processed_items / job.value.total_items) * 100) : 0))

const { run: cancel } = useAsyncAction(async () => {
  await useApi(`/import/${props.job.id}`, { method: 'DELETE' })
}, 'Import job cancelled.')
</script>

<template>
  <div class="bg-base-200 rounded-lg p-3">
    <div class="flex items-center justify-between">
      <div>
        <p class="text-sm font-medium">
          {{ job.filename }}
        </p>
        <p class="text-base-content/50 flex items-center gap-1.5 text-xs">
          <Icon :name="service.icon" size="11" /> {{ service.label }}
        </p>
      </div>
      <span class="badge badge-sm" :class="statusColor[job.status]">{{ job.status }}</span>
    </div>
    <progress
      v-if="job.status === 'running' || job.status === 'queued'"
      class="progress progress-primary mt-2 w-full"
      :value="progress"
      max="100"
    />
    <p class="text-base-content/50 mt-1 text-xs">
      {{ job.processed_items }}/{{ job.total_items }} processed · {{ job.imported_items }} imported · {{ job.skipped_items }} skipped · {{ job.failed_items }} failed
    </p>
    <p v-if="job.error_message" class="text-error mt-1 text-xs">
      {{ job.error_message }}
    </p>
    <ConfirmButton v-if="cancellable" class="btn btn-ghost btn-xs mt-2" label="Cancel" confirm-label="Really cancel?" @confirmed="cancel" />
  </div>
</template>
