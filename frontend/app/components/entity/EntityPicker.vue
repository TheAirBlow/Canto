<script setup lang="ts">
import type { SearchResult } from '~/types/api'

interface PickedEntity { id: number, name: string, image_url?: string }

const props = defineProps<{ kind: 'artist' | 'album' | 'song', excludeId?: number, placeholder?: string }>()
const emit = defineEmits<{ select: [PickedEntity] }>()

const query = ref('')
const debouncedQuery = refDebounced(query, 250)
const results = ref<PickedEntity[]>([])
const loading = ref(false)
const open = ref(false)

const rootRef = useTemplateRef('root')
onClickOutside(rootRef, () => (open.value = false))

watch(debouncedQuery, async (q) => {
  if (!q.trim()) {
    results.value = []
    return
  }
  loading.value = true
  try {
    const data = await useApi<SearchResult[]>('/search', { query: { q, type: props.kind, limit: 8 } })
    results.value = data
      .map(r => r.artist ?? r.album ?? r.song)
      .filter((e): e is NonNullable<typeof e> => e !== undefined && e.id !== props.excludeId)
  } finally {
    loading.value = false
  }
})

function pick(item: PickedEntity) {
  emit('select', item)
  query.value = ''
  results.value = []
  open.value = false
}
</script>

<template>
  <div ref="root" class="relative">
    <label class="input input-sm flex w-full items-center gap-2">
      <Icon name="fa6-solid:magnifying-glass" size="10" />
      <input v-model="query" type="text" class="grow" :placeholder="placeholder ?? `Search ${kind}s…`" @focus="open = true">
      <span v-if="loading" class="loading loading-spinner loading-xs" />
    </label>
    <Transition name="pop">
      <div v-if="open && query && results.length > 0" class="bg-base-200 border-base-300 rounded-box absolute z-20 mt-1 w-full border py-1 shadow-lg">
        <button
          v-for="r in results"
          :key="r.id"
          type="button"
          class="hover:bg-base-300 flex w-full items-center gap-2 px-2 py-1.5 text-left text-sm"
          @click="pick(r)"
        >
          <div class="avatar">
            <div class="bg-base-300 w-6 rounded">
              <img v-if="r.image_url" :src="r.image_url" :alt="r.name">
            </div>
          </div>
          <span class="truncate">{{ r.name }}</span>
        </button>
      </div>
    </Transition>
  </div>
</template>
