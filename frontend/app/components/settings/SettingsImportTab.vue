<script setup lang="ts">
import { ApiError } from '~/composables/useApi'
import type { ApiErrorBody, ImportJob, ImportService } from '~/types/api'

const services: ImportService[] = ['spotify', 'ytmusic', 'lastfm', 'listenbrainz', 'maloja', 'canto_export', 'koito']

const service = ref<ImportService>('spotify')
const files = ref<FileList | null>(null)
const fileInput = useTemplateRef('fileInputEl')
const jobs = ref<ImportJob[]>([])
const jobsLoaded = ref(false)
const jobsLoading = ref(true)
const jobsError = ref('')
const refreshing = ref(false)

async function loadJobs(silent = false) {
  if (!silent) jobsLoading.value = true
  else refreshing.value = true
  jobsError.value = ''
  try {
    jobs.value = await useApi<ImportJob[]>('/import')
    jobsLoaded.value = true
  } catch (err) {
    jobsError.value = err instanceof ApiError ? (err.body?.detail ?? err.message) : 'Failed to load import jobs.'
  } finally {
    jobsLoading.value = false
    refreshing.value = false
  }
}
loadJobs()
const jobsShown = useDelayedPending(computed(() => jobsLoading.value && !jobsLoaded.value))

const refreshTimer = setInterval(() => loadJobs(true), 5000)
onUnmounted(() => clearInterval(refreshTimer))

function onFileChange(event: Event) {
  files.value = (event.target as HTMLInputElement).files
}

const uploading = ref(false)
const uploadProgress = ref(0)

async function upload() {
  if (!files.value || files.value.length === 0) return
  const form = new FormData()
  form.append('service', service.value)
  for (const file of files.value) form.append('files[]', file)

  uploading.value = true
  uploadProgress.value = 0
  const toast = useToast()
  try {
    await new Promise<void>((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      xhr.open('POST', `${useRuntimeConfig().public.apiBase}/import`)
      xhr.withCredentials = true
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable) uploadProgress.value = Math.round((event.loaded / event.total) * 100)
      }
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          resolve()
          return
        }
        let body: ApiErrorBody | undefined
        try {
          body = JSON.parse(xhr.responseText)
        } catch {
          body = undefined
        }
        reject(new ApiError(xhr.status, body))
      }
      xhr.onerror = () => reject(new Error('Network error during upload.'))
      xhr.send(form)
    })
    toast.success('Import started.')
    files.value = null
    if (fileInput.value) fileInput.value.value = ''
    await loadJobs(true)
  } catch (err) {
    toast.error(err instanceof ApiError ? (err.body?.detail ?? err.message) : 'Something went wrong.')
  } finally {
    uploading.value = false
    uploadProgress.value = 0
  }
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <fieldset class="fieldset">
      <legend class="fieldset-legend">Service</legend>
      <IconSelect v-model="service" size="md" :options="services.map(s => ({ value: s, label: serviceMeta(s).label, icon: serviceMeta(s).icon }))" />
    </fieldset>
    <fieldset class="fieldset">
      <legend class="fieldset-legend">Files</legend>
      <input ref="fileInputEl" type="file" multiple class="file-input w-full" @change="onFileChange">
      <p v-if="files && files.length > 1" class="text-base-content/60 mt-1 text-xs">
        {{ files.length }} files selected
      </p>
    </fieldset>
    <button class="btn btn-primary" :class="{ 'btn-disabled': uploading || !files?.length }" @click="upload">
      <span v-if="uploading" class="loading loading-spinner loading-xs" /> Upload
    </button>
    <progress v-if="uploading" class="progress progress-primary w-full" :value="uploadProgress" max="100" />

    <div class="mt-2 flex items-center gap-3">
      <div class="divider mb-0 grow">
        Jobs
      </div>
      <button class="btn btn-ghost btn-xs btn-circle" aria-label="Refresh" :class="{ 'btn-disabled': refreshing }" @click="loadJobs(true)">
        <Icon name="fa6-solid:arrows-rotate" size="12" :class="{ 'animate-spin': refreshing }" />
      </button>
    </div>
    <Transition name="fade" mode="out-in">
      <div v-if="jobsShown" key="skeleton" class="flex flex-col gap-2">
        <div v-for="i in 3" :key="i" class="skeleton h-16 w-full" />
      </div>
      <p v-else-if="jobsError" key="error" class="text-error text-sm">
        {{ jobsError }} <button class="link" @click="loadJobs()">Retry</button>
      </p>
      <p v-else-if="jobs.length === 0" key="empty" class="text-base-content/60 text-sm">
        No import jobs yet.
      </p>
      <div v-else key="content" class="flex flex-col gap-2">
        <ImportJobRow v-for="job in jobs" :key="job.id" :job="job" />
      </div>
    </Transition>
  </div>
</template>
