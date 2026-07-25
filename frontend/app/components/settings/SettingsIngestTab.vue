<script setup lang="ts">
import { ApiError } from '~/composables/useApi'
import type { Settings, SettingsRegistry } from '~/types/api'

const settings = ref<Settings | null>(null)
const registry = ref<SettingsRegistry | null>(null)
const loading = ref(true)
const loadError = ref('')

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const [s, r] = await Promise.all([
      useApi<Settings>('/settings'),
      useApi<SettingsRegistry>('/settings/registry'),
    ])
    s.forwards ??= []
    settings.value = s
    registry.value = r
  } catch (err) {
    loadError.value = err instanceof ApiError ? (err.body?.detail ?? err.message) : 'Failed to load settings.'
  } finally {
    loading.value = false
  }
}
load()
const shown = useDelayedPending(loading)

const { loading: saving, run: save } = useAsyncAction(async () => {
  if (!settings.value) return
  settings.value = await useApi<Settings>('/settings', { method: 'PUT', body: settings.value })
}, 'Settings saved.')

const linkCandidates = computed(() => registry.value?.processors.filter(p => p.can_detect).map(p => ({ id: p.id, available: p.available })) ?? [])
const fallbackCandidates = computed(() => registry.value?.processors.filter(p => p.can_lookup).map(p => ({ id: p.id, available: p.available })) ?? [])
const matcherCandidates = computed(() => registry.value?.matchers.map(m => ({ id: m.id, available: m.available })) ?? [])

function toggleIngester(id: string) {
  if (!settings.value) return
  const idx = settings.value.ingesters.indexOf(id)
  if (idx === -1) settings.value.ingesters.push(id)
  else settings.value.ingesters.splice(idx, 1)
}

const requestUrl = useRequestURL()
function ingesterUrl(apiPath: string) {
  return `${requestUrl.origin}${useRuntimeConfig().public.apiBase}${apiPath}`
}

const copiedId = ref('')
async function copyUrl(id: string, url: string) {
  await navigator.clipboard.writeText(url)
  copiedId.value = id
  setTimeout(() => { if (copiedId.value === id) copiedId.value = '' }, 1500)
}

function addForward() {
  settings.value?.forwards.push({ ingester: registry.value?.ingesters[0]?.id ?? '', type: 'listenbrainz', url: '', token: '' })
}

function removeForward(i: number) {
  settings.value?.forwards.splice(i, 1)
}
</script>

<template>
  <Transition name="fade" mode="out-in">
    <div v-if="shown" key="skeleton" class="flex flex-col gap-3">
      <div class="skeleton h-6 w-32" />
      <div class="skeleton h-24 w-full" />
      <div class="skeleton h-24 w-full" />
    </div>
    <p v-else-if="loadError" key="error" class="text-error text-sm">
      {{ loadError }} <button class="link" @click="load">Retry</button>
    </p>
    <div v-else-if="settings && registry" key="content" class="flex flex-col gap-6">
    <div>
      <h4 class="mb-2 text-sm font-semibold">
        Ingest endpoints
      </h4>
      <div v-for="ingester in registry.ingesters" :key="ingester.id" class="bg-base-200 mb-2 flex flex-col gap-2 rounded-lg p-3">
        <label class="label cursor-pointer justify-start gap-2">
          <input type="checkbox" class="toggle toggle-sm toggle-primary" :checked="settings.ingesters.includes(ingester.id)" @change="toggleIngester(ingester.id)">
          <span class="text-sm font-medium">{{ ingester.label }}</span>
        </label>
        <div class="join">
          <input type="text" readonly class="input input-sm join-item flex-1 font-mono text-xs" :value="ingesterUrl(ingester.api_path)">
          <button class="btn btn-sm join-item" @click="copyUrl(ingester.id, ingesterUrl(ingester.api_path))">
            <Icon :name="copiedId === ingester.id ? 'fa6-solid:check' : 'fa6-solid:copy'" size="12" />
          </button>
        </div>
      </div>
    </div>

    <div>
      <h4 class="mb-2 text-sm font-semibold">
        Link processor order
      </h4>
      <SettingsRegistryList v-model="settings.link_processors" :available="linkCandidates" />
    </div>

    <div>
      <h4 class="mb-2 text-sm font-semibold">
        Fallback processor order
      </h4>
      <SettingsRegistryList v-model="settings.fallback_processors" :available="fallbackCandidates" />
    </div>

    <div>
      <h4 class="mb-2 text-sm font-semibold">
        Fuzzy matcher order
      </h4>
      <SettingsRegistryList v-model="settings.fuzzy_matchers" :available="matcherCandidates" />
    </div>

    <div>
      <div class="mb-2 flex items-center justify-between">
        <h4 class="text-sm font-semibold">
          Forwarding rules
        </h4>
        <button class="btn btn-ghost btn-xs" @click="addForward">
          <Icon name="fa6-solid:plus" size="10" /> Add
        </button>
      </div>
      <div v-if="settings.forwards.length === 0" class="text-base-content/60 text-sm">
        No forwarding rules configured.
      </div>
      <div v-for="(fwd, i) in settings.forwards" :key="i" class="bg-base-200 mb-2 flex flex-col gap-2 rounded-lg p-3">
        <div class="flex gap-2">
          <IconSelect
            v-model="fwd.ingester"
            :options="registry.ingesters.map(ingester => ({ value: ingester.id, label: ingester.label, icon: serviceMeta(ingester.id).icon }))"
          />
          <button class="btn btn-ghost btn-xs text-error" @click="removeForward(i)">
            <Icon name="fa6-solid:trash" size="12" />
          </button>
        </div>
        <input v-model="fwd.type" type="text" placeholder="Target type (e.g. listenbrainz, lastfm)" class="input input-sm">
        <input v-model="fwd.url" type="text" placeholder="Server URL" class="input input-sm">
        <input v-model="fwd.token" type="text" placeholder="Token" class="input input-sm">
      </div>
    </div>

      <button class="btn btn-primary" :class="{ 'btn-disabled': saving }" @click="save">
        <span v-if="saving" class="loading loading-spinner loading-xs" /> Save
      </button>
    </div>
  </Transition>
</template>
