<script setup lang="ts">
const props = defineProps<{
  items: { id: number, name: string, imageUrl?: string }[]
  kind: 'artist' | 'album' | 'song'
  title: string
}>()

const expanded = ref(false)

const scroller = ref<HTMLElement>()
const atStart = ref(true)
const atEnd = ref(false)

function updateEdges() {
  const el = scroller.value
  if (!el || expanded.value) return
  atStart.value = el.scrollLeft <= 1
  atEnd.value = el.scrollLeft >= el.scrollWidth - el.clientWidth - 1
}

function scrollByPage(dir: 1 | -1) {
  scroller.value?.scrollBy({ left: dir * scroller.value.clientWidth * 0.9, behavior: 'smooth' })
}

// Animates the container's height across the nowrap/wrap toggle, since `height: auto`
// isn't transitionable — locks the pre-toggle height, flips the wrap mode, then
// transitions to the newly measured height and clears back to auto once settled.
function toggleExpanded() {
  const el = scroller.value
  if (!el) {
    expanded.value = !expanded.value
    return
  }

  el.style.height = `${el.offsetHeight}px`
  expanded.value = !expanded.value

  nextTick(() => {
    const target = el.scrollHeight
    el.style.transition = 'height 0.3s ease'
    requestAnimationFrame(() => { el.style.height = `${target}px` })
    el.addEventListener('transitionend', () => {
      el.style.height = ''
      el.style.transition = ''
      updateEdges()
    }, { once: true })
  })
}

useResizeObserver(scroller, updateEdges)
watch(() => props.items, () => nextTick(updateEdges))
onMounted(() => nextTick(updateEdges))
</script>

<template>
  <div>
    <div class="mb-3 flex items-center gap-2">
      <button
        type="button"
        class="btn btn-ghost btn-xs btn-circle"
        :aria-expanded="expanded"
        :aria-label="expanded ? 'Collapse' : 'Expand'"
        @click="toggleExpanded"
      >
        <Icon name="fa6-solid:chevron-right" size="12" class="transition-transform duration-200" :class="{ 'rotate-90': expanded }" />
      </button>
      <h2 class="flex-1 text-lg font-bold">
        {{ title }}
      </h2>
      <div
        class="flex gap-1 transition-opacity duration-200"
        :class="expanded ? 'pointer-events-none opacity-0' : 'opacity-100'"
      >
        <button
          type="button"
          class="btn btn-circle btn-sm"
          :disabled="atStart"
          aria-label="Scroll left"
          @click="scrollByPage(-1)"
        >
          <Icon name="fa6-solid:chevron-left" size="12" />
        </button>
        <button
          type="button"
          class="btn btn-circle btn-sm"
          :disabled="atEnd"
          aria-label="Scroll right"
          @click="scrollByPage(1)"
        >
          <Icon name="fa6-solid:chevron-right" size="12" />
        </button>
      </div>
    </div>

    <div
      ref="scroller"
      class="flex gap-4 overflow-x-auto overflow-y-hidden scroll-smooth pb-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      :class="expanded ? 'flex-wrap' : 'flex-nowrap snap-x snap-mandatory'"
      @scroll="updateEdges"
    >
      <div v-for="item in items" :key="item.id" class="w-32 shrink-0 snap-start sm:w-40">
        <MediaItemCard :id="item.id" :kind="kind" :name="item.name" :image-url="item.imageUrl" />
      </div>
    </div>
  </div>
</template>
