<script setup lang="ts">
import { ApiError } from '~/composables/useApi'
import type { ApiKey, ApiKeyCreated } from '~/types/api'

const keys = ref<ApiKey[]>([])
const loading = ref(true)
const loadError = ref('')
const newName = ref('')
const revealedKey = ref<string | null>(null)

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    keys.value = await useApi<ApiKey[]>('/keys')
  } catch (err) {
    loadError.value = err instanceof ApiError ? (err.body?.detail ?? err.message) : 'Failed to load API keys.'
  } finally {
    loading.value = false
  }
}
load()
const shown = useDelayedPending(loading)

const { loading: creating, run: create } = useAsyncAction(async () => {
  if (!newName.value.trim()) return
  const created = await useApi<ApiKeyCreated>('/keys', { method: 'POST', body: { name: newName.value } })
  revealedKey.value = created.key
  newName.value = ''
  await load()
})

const { run: remove } = useAsyncAction(async (id: number) => {
  await useApi(`/keys/${id}`, { method: 'DELETE' })
  await load()
}, 'API key deleted.')
</script>

<template>
  <div class="flex flex-col gap-4">
    <p class="text-base-content/60 text-xs">
      Use an API key with a ListenBrainz-compatible scrobbler to submit listens from another app.
    </p>

    <div v-if="revealedKey" class="alert alert-warning flex-col items-start gap-2">
      <p class="text-sm font-medium">
        Copy this key now — it won't be shown again.
      </p>
      <code class="bg-base-100 w-full break-all rounded p-2 text-xs">{{ revealedKey }}</code>
      <button class="btn btn-xs" @click="revealedKey = null">
        Done
      </button>
    </div>

    <div class="flex gap-2">
      <input v-model="newName" type="text" placeholder="Key name" class="input input-sm flex-1">
      <button class="btn btn-sm btn-primary" :class="{ 'btn-disabled': creating || !newName.trim() }" @click="create">
        Create
      </button>
    </div>

    <Transition name="fade" mode="out-in">
      <div v-if="shown" key="skeleton" class="skeleton h-20 w-full" />
      <p v-else-if="loadError" key="error" class="text-error text-sm">
        {{ loadError }} <button class="link" @click="load">Retry</button>
      </p>
      <p v-else-if="keys.length === 0" key="empty" class="text-base-content/60 text-sm">
        No API keys yet.
      </p>
      <div v-else key="content" class="flex flex-col gap-2">
        <div v-for="key in keys" :key="key.id" class="bg-base-200 flex items-center justify-between rounded-lg p-3">
          <div>
            <p class="font-medium">
              {{ key.name }}
            </p>
            <p class="text-base-content/50 text-xs">
              {{ key.last_used_at ? `Last used ${new Date(key.last_used_at).toLocaleDateString()}` : 'Never used' }}
            </p>
          </div>
          <ConfirmButton v-slot="{ confirming }" class="btn btn-ghost btn-xs text-error" @confirmed="remove(key.id)">
            <Icon :name="confirming ? 'fa6-solid:check' : 'fa6-solid:trash'" size="12" />
            <span v-if="confirming">Really?</span>
          </ConfirmButton>
        </div>
      </div>
    </Transition>
  </div>
</template>
