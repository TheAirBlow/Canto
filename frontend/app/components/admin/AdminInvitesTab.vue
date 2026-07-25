<script setup lang="ts">
import { ApiError } from '~/composables/useApi'
import type { Invite } from '~/types/api'

const invites = ref<Invite[]>([])
const loading = ref(true)
const loadError = ref('')
const newName = ref('')
const maxUses = ref<number | null>(null)

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    invites.value = await useApi<Invite[]>('/admin/invites')
  } catch (err) {
    loadError.value = err instanceof ApiError ? (err.body?.detail ?? err.message) : 'Failed to load invites.'
  } finally {
    loading.value = false
  }
}
load()
const shown = useDelayedPending(loading)

const { loading: creating, run: create } = useAsyncAction(async () => {
  if (!newName.value.trim()) return
  await useApi('/admin/invites', { method: 'POST', body: { name: newName.value, max_uses: maxUses.value || undefined } })
  newName.value = ''
  maxUses.value = null
  await load()
}, 'Invite created.')

const { run: remove } = useAsyncAction(async (id: number) => {
  await useApi(`/admin/invites/${id}`, { method: 'DELETE' })
  await load()
}, 'Invite deleted.')

const toast = useToast()
function copyCode(code: string) {
  navigator.clipboard?.writeText(code)
  toast.success('Copied to clipboard.')
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <div class="flex gap-2">
      <input v-model="newName" type="text" placeholder="Invite name" class="input input-sm flex-1">
      <input v-model.number="maxUses" type="number" placeholder="Max uses" class="input input-sm w-24" min="1">
      <button class="btn btn-sm btn-primary" :class="{ 'btn-disabled': creating || !newName.trim() }" @click="create">
        Create
      </button>
    </div>

    <Transition name="fade" mode="out-in">
      <div v-if="shown" key="skeleton" class="skeleton h-20 w-full" />
      <p v-else-if="loadError" key="error" class="text-error text-sm">
        {{ loadError }} <button class="link" @click="load">Retry</button>
      </p>
      <p v-else-if="invites.length === 0" key="empty" class="text-base-content/60 text-sm">
        No invites yet.
      </p>
      <div v-else key="content" class="flex flex-col gap-2">
        <div v-for="invite in invites" :key="invite.id" class="bg-base-200 flex items-center justify-between rounded-lg p-3">
          <div>
            <p class="font-medium">
              {{ invite.name }}
            </p>
            <div class="flex items-center gap-2">
              <code class="text-xs">{{ invite.code }}</code>
              <button class="btn btn-ghost btn-xs" @click="copyCode(invite.code)">
                <Icon name="fa6-solid:copy" size="10" />
              </button>
            </div>
            <p class="text-base-content/50 text-xs">
              {{ invite.uses_count }}{{ invite.max_uses ? ` / ${invite.max_uses}` : '' }} uses
            </p>
          </div>
          <ConfirmButton v-slot="{ confirming }" class="btn btn-ghost btn-xs text-error" @confirmed="remove(invite.id)">
            <Icon :name="confirming ? 'fa6-solid:check' : 'fa6-solid:trash'" size="12" />
            <span v-if="confirming">Really?</span>
          </ConfirmButton>
        </div>
      </div>
    </Transition>
  </div>
</template>
