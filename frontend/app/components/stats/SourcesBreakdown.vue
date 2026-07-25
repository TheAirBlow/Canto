<script setup lang="ts">
import type { SourceEntry } from '~/types/api'

const props = defineProps<{ sources: SourceEntry[] }>()

const palette = [
  'var(--color-primary)',
  'var(--color-secondary)',
  'var(--color-accent)',
  'var(--color-info)',
  'var(--color-success)',
  'var(--color-warning)',
]

// DonutChart's tooltip looks up values by category name (not key), so key by the
// display label to match — see vue-chrts DonutChart.js's segment handler.
const categories = computed(() => Object.fromEntries(
  props.sources.map((s, i) => [serviceMeta(s.source_type).label, { name: serviceMeta(s.source_type).label, color: palette[i % palette.length]! }]),
))
const data = computed(() => props.sources.map(s => s.listen_count))
</script>

<template>
  <div>
    <p v-if="sources.length === 0" class="text-base-content/60 text-sm">
      No source data yet.
    </p>
    <DonutChart v-else :data="data" :categories="categories" :radius="80" :height="220" :duration="0" />
  </div>
</template>
