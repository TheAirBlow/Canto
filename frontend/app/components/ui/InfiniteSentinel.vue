<script setup lang="ts">
const props = defineProps<{ disabled?: boolean }>()
const emit = defineEmits<{ load: [] }>()

const target = useTemplateRef('target')

useIntersectionObserver(
  target,
  ([entry]) => {
    if (entry?.isIntersecting && !props.disabled) emit('load')
  },
  { rootMargin: '400px' },
)

// Fallback for cases where IntersectionObserver misses a layout change above the sentinel
// (e.g. async content loading in) and only recovers on the next resize.
function checkVisible() {
  if (props.disabled || !target.value) return
  if (target.value.getBoundingClientRect().top <= window.innerHeight + 400) emit('load')
}
useEventListener(window, 'scroll', checkVisible, { passive: true })
useEventListener(window, 'resize', checkVisible)
onMounted(() => nextTick(checkVisible))
</script>

<template>
  <div ref="target" class="h-px w-full" />
</template>
