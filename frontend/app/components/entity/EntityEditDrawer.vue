<script setup lang="ts">
import { ApiError } from '~/composables/useApi'
import type { Alias } from '~/types/api'

type EntityKind = 'artist' | 'album' | 'song'
interface LinkedArtist { id: number, name: string, image_url?: string }

const props = defineProps<{ kind: EntityKind, id: number }>()
const emit = defineEmits<{ updated: [], deleted: [] }>()

interface EditableEntity {
  id: number
  name: string
  description?: string
  pinned: boolean
  image_url?: string
  artists?: LinkedArtist[]
}

const entity = ref<EditableEntity | null>(null)
const name = ref('')
const description = ref('')
const fileInput = useTemplateRef('fileInput')
const loadError = ref('')

async function load() {
  loadError.value = ''
  try {
    const data = await useApi<EditableEntity>(`/${props.kind}s/${props.id}`)
    entity.value = data
    name.value = data.name
    description.value = data.description ?? ''
  } catch (err) {
    loadError.value = err instanceof ApiError ? (err.body?.detail ?? err.message) : 'Failed to load.'
  }
}
load()
const shown = useDelayedPending(computed(() => !entity.value && !loadError.value))

const { loading: saving, run: save } = useAsyncAction(async () => {
  await useApi(`/${props.kind}s/${props.id}`, { method: 'PUT', body: { name: name.value, description: description.value || undefined } })
  await load()
  emit('updated')
}, 'Saved.')

const toast = useToast()
const { run: togglePin } = useAsyncAction(async () => {
  if (!entity.value) return
  const wasPinned = entity.value.pinned
  await useApi(`/${props.kind}s/${props.id}/pin`, { method: wasPinned ? 'DELETE' : 'PUT' })
  await load()
  emit('updated')
  toast.success(wasPinned ? 'Unpinned.' : 'Pinned.')
})

const mergeTarget = ref<{ id: number, name: string } | null>(null)
const { loading: merging, run: merge } = useAsyncAction(async () => {
  if (!mergeTarget.value) return
  await useApi(`/${props.kind}s/${props.id}/merge`, { method: 'POST', body: { into: mergeTarget.value.id } })
  emit('updated')
}, 'Merged.')

const { loading: uploading, run: uploadImage } = useAsyncAction(async (file: File) => {
  const form = new FormData()
  form.append('image', file)
  await useApi(`/${props.kind}s/${props.id}/image`, { method: 'PUT', body: form })
  await load()
}, 'Image updated.')

function onFileChange(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (file) uploadImage(file)
}

const dropActive = ref(false)
function onDrop(event: DragEvent) {
  dropActive.value = false
  const file = event.dataTransfer?.files[0]
  if (file?.type.startsWith('image/')) uploadImage(file)
}

// Aliases
const aliases = ref<Alias[]>([])
const newAlias = ref('')
async function loadAliases() {
  aliases.value = await useApi<Alias[]>(`/${props.kind}s/${props.id}/aliases`)
}
loadAliases()

const { loading: addingAlias, run: addAlias } = useAsyncAction(async () => {
  if (!newAlias.value.trim()) return
  await useApi(`/${props.kind}s/${props.id}/aliases`, { method: 'POST', body: { alias: newAlias.value } })
  newAlias.value = ''
  await loadAliases()
})

const { run: removeAlias } = useAsyncAction(async (aliasId: number) => {
  await useApi(`/${props.kind}s/${props.id}/aliases/${aliasId}`, { method: 'DELETE' })
  await loadAliases()
})

// Linked artists (albums/songs only) — first entry is the primary artist.
const linkedArtists = ref<LinkedArtist[]>([])
watch(entity, (e) => { linkedArtists.value = e?.artists ? [...e.artists] : [] })

function addArtist(picked: LinkedArtist) {
  if (linkedArtists.value.some(a => a.id === picked.id)) return
  linkedArtists.value.push(picked)
}
function removeArtistAt(idx: number) {
  linkedArtists.value.splice(idx, 1)
}
function moveArtist(idx: number, dir: -1 | 1) {
  const swapWith = idx + dir
  if (swapWith < 0 || swapWith >= linkedArtists.value.length) return
  const v = linkedArtists.value
  ;[v[idx], v[swapWith]] = [v[swapWith]!, v[idx]!]
}

const { loading: savingArtists, run: saveArtists } = useAsyncAction(async () => {
  if (linkedArtists.value.length === 0) return
  await useApi(`/${props.kind}s/${props.id}/artists`, {
    method: 'PUT',
    body: { artist_ids: linkedArtists.value.map(a => a.id) },
  })
  await load()
  emit('updated')
}, 'Artists updated.')

// Deletion
const deleteModalOpen = ref(false)
const { loading: deleting, run: deleteEntity } = useAsyncAction(async () => {
  await useApi(`/${props.kind}s/${props.id}`, { method: 'DELETE' })
  deleteModalOpen.value = false
  emit('deleted')
}, 'Deleted.')
</script>

<template>
  <Transition name="fade" mode="out-in">
    <p v-if="loadError" key="error" class="text-error text-sm">
      {{ loadError }} <button class="link" @click="load">Retry</button>
    </p>
    <div v-else-if="shown" key="skeleton" class="flex flex-col gap-3">
      <div class="skeleton h-14 w-full" />
      <div class="skeleton h-20 w-full" />
    </div>
    <div v-else-if="entity" key="content" class="flex flex-col gap-4">
      <div
        class="rounded-box flex items-center gap-3 border-2 border-dashed p-2 transition-colors"
        :class="dropActive ? 'border-primary bg-primary/5' : 'border-transparent'"
        @dragover.prevent="dropActive = true"
        @dragleave.prevent="dropActive = false"
        @drop.prevent="onDrop"
      >
        <div class="avatar">
          <div class="bg-base-300 w-14 rounded">
            <img v-if="entity.image_url" :src="entity.image_url" :alt="entity.name">
          </div>
        </div>
        <div class="flex flex-col gap-1">
          <button class="btn btn-xs w-fit" :class="{ 'btn-disabled': uploading }" @click="fileInput?.click()">
            <span v-if="uploading" class="loading loading-spinner loading-xs" /> Replace image
          </button>
          <span class="text-base-content/50 text-xs">or drag an image here</span>
        </div>
        <input ref="fileInput" type="file" accept="image/*" class="hidden" @change="onFileChange">
      </div>

      <fieldset class="fieldset">
        <legend class="fieldset-legend">Name</legend>
        <input v-model="name" type="text" class="input w-full">
      </fieldset>
      <fieldset class="fieldset">
        <legend class="fieldset-legend">Description</legend>
        <textarea v-model="description" class="textarea w-full" rows="2" />
      </fieldset>
      <button class="btn btn-primary btn-sm" :class="{ 'btn-disabled': saving }" @click="save">
        Save
      </button>

      <div class="divider" />
      <button class="btn btn-outline btn-sm" @click="togglePin">
        <Icon name="fa6-solid:thumbtack" size="12" /> {{ entity.pinned ? 'Unpin' : 'Pin' }}
      </button>

      <template v-if="kind !== 'artist'">
        <div class="divider">
          Artists
        </div>
        <p class="text-base-content/60 text-xs">
          First artist is the primary credit.
        </p>
        <div class="flex flex-col gap-1">
          <div v-for="(a, idx) in linkedArtists" :key="a.id" class="join w-full">
            <span class="join-item bg-base-100 border-base-300 flex flex-1 items-center gap-2 border px-3 py-1.5 text-sm">
              {{ a.name }}
              <span v-if="idx === 0" class="badge badge-primary badge-xs normal-case">primary</span>
            </span>
            <button class="btn btn-ghost join-item px-2" :disabled="idx === 0" @click="moveArtist(idx, -1)">
              <Icon name="fa6-solid:chevron-up" size="10" />
            </button>
            <button class="btn btn-ghost join-item px-2" :disabled="idx === linkedArtists.length - 1" @click="moveArtist(idx, 1)">
              <Icon name="fa6-solid:chevron-down" size="10" />
            </button>
            <button class="btn btn-ghost join-item text-error px-2" :disabled="linkedArtists.length <= 1" @click="removeArtistAt(idx)">
              <Icon name="fa6-solid:trash" size="10" />
            </button>
          </div>
        </div>
        <EntityPicker kind="artist" placeholder="Add an artist…" @select="addArtist" />
        <button class="btn btn-sm" :class="{ 'btn-disabled': savingArtists }" @click="saveArtists">
          <span v-if="savingArtists" class="loading loading-spinner loading-xs" /> Save artists
        </button>
      </template>

      <div class="divider">
        Aliases
      </div>
      <div class="flex flex-col gap-1">
        <div v-for="alias in aliases" :key="alias.id" class="bg-base-100 border-base-300 flex items-center justify-between rounded-lg border px-3 py-1.5 text-sm">
          {{ alias.alias }}
          <button class="btn btn-ghost btn-xs text-error" @click="removeAlias(alias.id)">
            <Icon name="fa6-solid:trash" size="10" />
          </button>
        </div>
        <p v-if="aliases.length === 0" class="text-base-content/60 text-xs">
          No aliases yet.
        </p>
      </div>
      <div class="join w-full">
        <input v-model="newAlias" type="text" placeholder="Add an alias" class="input input-sm join-item flex-1" @keyup.enter="addAlias">
        <button class="btn btn-sm join-item" :class="{ 'btn-disabled': addingAlias || !newAlias.trim() }" @click="addAlias">
          <Icon name="fa6-solid:plus" size="10" /> Add
        </button>
      </div>

      <div class="divider">
        Merge
      </div>
      <p class="text-base-content/60 text-xs">
        Merge this {{ kind }} into another one. This one will be deleted and every reference repointed.
      </p>
      <EntityPicker :kind="kind" :exclude-id="id" @select="mergeTarget = $event" />
      <div v-if="mergeTarget" class="flex items-center justify-between gap-2">
        <span class="text-sm">
          Merge into <span class="font-medium">{{ mergeTarget.name }}</span>
        </span>
        <ConfirmButton
          class="btn btn-sm btn-error"
          label="Merge"
          confirm-label="Really merge?"
          :disabled="merging"
          @confirmed="merge"
        />
      </div>

      <div class="divider">
        Danger zone
      </div>
      <p class="text-base-content/60 text-xs">
        Permanently delete this {{ kind }}{{ kind === 'song' ? ', including every listen of it' : '' }}.
      </p>
      <button class="btn btn-outline btn-error btn-sm w-fit" @click="deleteModalOpen = true">
        <Icon name="fa6-solid:trash" size="12" /> Delete this {{ kind }}
      </button>

      <AppModal v-if="deleteModalOpen" title="This action is irreversible" @close="deleteModalOpen = false">
        <p class="text-sm">
          Delete <span class="font-semibold">{{ entity.name }}</span>?
          <template v-if="kind === 'song'">
            Every listen of this track disappears from history too — not just the catalog entry.
          </template>
          <template v-else>
            This only removes the {{ kind }} and its associations; its songs are not deleted.
          </template>
          There is no undo.
        </p>
        <div class="mt-4 flex justify-end gap-2">
          <button class="btn btn-sm" @click="deleteModalOpen = false">
            Cancel
          </button>
          <button class="btn btn-sm btn-error" :class="{ 'btn-disabled': deleting }" @click="deleteEntity">
            <span v-if="deleting" class="loading loading-spinner loading-xs" /> Delete permanently
          </button>
        </div>
      </AppModal>
    </div>
  </Transition>
</template>
