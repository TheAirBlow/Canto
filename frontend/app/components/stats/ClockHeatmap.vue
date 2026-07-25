<script setup lang="ts">
import type { ClockCell } from '~/types/api'

const props = defineProps<{ cells: ClockCell[] }>()

const { level } = useHeatmapScale(computed(() => props.cells.map(c => c.listen_count)))
const levelClass = ['bg-base-300', 'bg-primary/25', 'bg-primary/50', 'bg-primary/75', 'bg-primary']
const dayLabels = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const hours = Array.from({ length: 24 }, (_, i) => i)
const hourLabels: Record<number, string> = { 0: '12a', 6: '6a', 12: '12p', 18: '6p' }

const countByCell = computed(() => new Map(props.cells.map(c => [`${c.day_of_week}-${c.hour}`, c.listen_count])))
function countFor(day: number, hour: number) {
  return countByCell.value.get(`${day}-${hour}`) ?? 0
}

// Cell size is measured from the grid's own (capped) width rather than derived via aspect-ratio,
// so cells stay true squares; capping the wrapper below keeps it from ballooning to the full card.
const gap = 3
const gridRef = useTemplateRef('grid')
const { width: gridWidth } = useElementSize(gridRef)
const cellSize = computed(() => Math.max(0, (gridWidth.value - gap * (hours.length - 1)) / hours.length))
</script>

<template>
  <div class="max-w-lg">
    <div class="flex flex-col" :style="{ gap: `${gap}px` }">
      <div class="flex items-center" :style="{ gap: `${gap}px` }">
        <span class="w-6 shrink-0" />
        <div ref="grid" class="flex flex-1" :style="{ gap: `${gap}px` }">
          <span
            v-for="hour in hours"
            :key="hour"
            class="text-base-content/40 shrink-0 text-center text-[8px] leading-none"
            :style="{ width: `${cellSize}px` }"
          >
            {{ hourLabels[hour] ?? '' }}
          </span>
        </div>
      </div>
      <div v-for="(dayLabel, d) in dayLabels" :key="d" class="flex items-center" :style="{ gap: `${gap}px` }">
        <span class="text-base-content/50 w-6 shrink-0 pr-1 text-right text-[9px]">{{ dayLabel }}</span>
        <div class="flex flex-1" :style="{ gap: `${gap}px` }">
          <div
            v-for="hour in hours"
            :key="hour"
            class="group/cell relative shrink-0"
            :style="{ width: `${cellSize}px`, height: `${cellSize}px` }"
          >
            <button
              type="button"
              class="h-full w-full rounded-[2px] transition-colors focus-visible:outline-primary focus-visible:outline-2 focus-visible:outline-offset-1"
              :class="levelClass[level(countFor(d, hour))]"
            />
            <div
              class="bg-base-100 border-base-300 text-base-content pointer-events-none absolute bottom-full left-1/2 z-20 mb-1.5 -translate-x-1/2 rounded-lg border px-2.5 py-1.5 text-xs whitespace-nowrap opacity-0 shadow-lg transition-opacity group-hover/cell:opacity-100 group-focus/cell:opacity-100"
            >
              <p class="font-medium">
                {{ dayLabel }} {{ hour }}:00
              </p>
              <p class="text-base-content/60">
                {{ countFor(d, hour) }} listen{{ countFor(d, hour) === 1 ? '' : 's' }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
