<script setup lang="ts">
import type { ActivityBucket } from '~/types/api'

const props = defineProps<{ buckets: ActivityBucket[], from: Date, to: Date }>()

const weeks = computed(() => calendarWeeks(props.buckets, props.from, props.to))
const caption = computed(() => calendarRangeCaption(weeks.value))

const { level } = useHeatmapScale(computed(() => props.buckets.map(b => b.listen_count)))
const levelClass = ['bg-base-300', 'bg-primary/25', 'bg-primary/50', 'bg-primary/75', 'bg-primary']

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric', timeZone: 'UTC' })
}

// Cell size is measured, not left to `aspect-square`: aspect-ratio on a flex-basis:0 item's
// main axis doesn't reliably resolve to a true square in every browser, so compute an
// explicit equal width/height in px from the row's own measured width instead.
const gap = 3
const rowRef = useTemplateRef('row')
const { width: rowWidth } = useElementSize(rowRef)
const cellSize = computed(() => {
  const n = weeks.value.length || 1
  return Math.max(0, (rowWidth.value - gap * (n - 1)) / n)
})
</script>

<template>
  <div>
    <div ref="row" class="flex w-full" :style="{ gap: `${gap}px` }">
      <div v-for="(week, wi) in weeks" :key="wi" class="flex flex-col" :style="{ gap: `${gap}px` }">
        <div
          v-for="cell in week"
          :key="cell.date"
          class="group/cell relative"
          :style="{ width: `${cellSize}px`, height: `${cellSize}px` }"
        >
          <button
            type="button"
            class="h-full w-full rounded-[2px] transition-colors focus-visible:outline-primary focus-visible:outline-2 focus-visible:outline-offset-1"
            :class="cell.inRange ? levelClass[level(cell.count)] : 'invisible pointer-events-none'"
            :disabled="!cell.inRange"
          />
          <div
            v-if="cell.inRange"
            class="bg-base-100 border-base-300 text-base-content pointer-events-none absolute bottom-full left-1/2 z-20 mb-1.5 -translate-x-1/2 rounded-lg border px-2.5 py-1.5 text-xs whitespace-nowrap opacity-0 shadow-lg transition-opacity group-hover/cell:opacity-100 group-focus/cell:opacity-100"
          >
            <p class="font-medium">
              {{ formatDate(cell.date) }}
            </p>
            <p class="text-base-content/60">
              {{ cell.count }} listen{{ cell.count === 1 ? '' : 's' }} · {{ Math.round(cell.minutes) }} min
            </p>
          </div>
        </div>
      </div>
    </div>
    <p class="text-base-content/40 mt-1.5 text-center text-[10px]">
      {{ caption }}
    </p>
  </div>
</template>
