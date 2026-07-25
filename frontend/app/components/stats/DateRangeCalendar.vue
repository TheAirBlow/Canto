<script setup lang="ts">
const props = defineProps<{ initialFrom?: Date, initialTo?: Date }>()
const emit = defineEmits<{ select: [from: Date, to: Date], reset: [] }>()

const monthNames = ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December']
const weekdayLabels = ['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su']

function startOfDay(d: Date) {
  const c = new Date(d)
  c.setHours(0, 0, 0, 0)
  return c
}
function sameDay(a: Date, b: Date) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate()
}

const today = startOfDay(new Date())
const viewMonth = ref(new Date((props.initialFrom ?? today).getFullYear(), (props.initialFrom ?? today).getMonth(), 1))

const rangeStart = ref<Date | null>(props.initialFrom ? startOfDay(props.initialFrom) : null)
const rangeEnd = ref<Date | null>(props.initialTo ? startOfDay(props.initialTo) : null)

const rangeLabel = computed(() => {
  if (!rangeStart.value || !rangeEnd.value) return 'Pick a start date'
  const fmt = (d: Date) => d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  return sameDay(rangeStart.value, rangeEnd.value) ? fmt(rangeStart.value) : `${fmt(rangeStart.value)} – ${fmt(rangeEnd.value)}`
})

function prevMonth() {
  viewMonth.value = new Date(viewMonth.value.getFullYear(), viewMonth.value.getMonth() - 1, 1)
}
function nextMonth() {
  viewMonth.value = new Date(viewMonth.value.getFullYear(), viewMonth.value.getMonth() + 1, 1)
}

// viewYear/viewMonthIndex back the header's jump-to controls, so reaching a distant month doesn't take dozens of prev/next clicks.
const viewYear = computed({
  get: () => viewMonth.value.getFullYear(),
  set: (y: number) => { viewMonth.value = new Date(y, viewMonth.value.getMonth(), 1) },
})
const viewMonthIndex = computed({
  get: () => viewMonth.value.getMonth(),
  set: (m: number) => { viewMonth.value = new Date(viewMonth.value.getFullYear(), m, 1) },
})

const MIN_YEAR = 1900

const yearInput = ref(String(viewYear.value))
watch(viewYear, (y) => { yearInput.value = String(y) })

function commitYear() {
  const n = Number.parseInt(yearInput.value, 10)
  if (Number.isFinite(n) && n >= MIN_YEAR && n <= today.getFullYear()) viewYear.value = n
  else yearInput.value = String(viewYear.value)
}

interface DayCell {
  date: Date
  inMonth: boolean
  inRange: boolean
  isStart: boolean
  isEnd: boolean
  isToday: boolean
  roundingClass: string
}

const effectiveRange = computed(() => {
  if (!rangeStart.value || !rangeEnd.value) return null
  return rangeEnd.value < rangeStart.value ? { start: rangeEnd.value, end: rangeStart.value } : { start: rangeStart.value, end: rangeEnd.value }
})

const weeks = computed<DayCell[][]>(() => {
  const y = viewMonth.value.getFullYear()
  const m = viewMonth.value.getMonth()
  const startOffset = (new Date(y, m, 1).getDay() + 6) % 7
  const gridStart = new Date(y, m, 1 - startOffset)
  const range = effectiveRange.value

  const cells: DayCell[] = []
  for (let i = 0; i < 42; i++) {
    const d = new Date(gridStart.getFullYear(), gridStart.getMonth(), gridStart.getDate() + i)
    const inRange = !!range && d >= range.start && d <= range.end
    cells.push({
      date: d,
      inMonth: d.getMonth() === m,
      inRange,
      isStart: inRange && sameDay(d, range!.start),
      isEnd: inRange && sameDay(d, range!.end),
      isToday: sameDay(d, today),
      roundingClass: '',
    })
  }
  const out: DayCell[][] = []
  for (let i = 0; i < cells.length; i += 7) out.push(cells.slice(i, i + 7))

  // Multi-row ranges round like a text-selection highlight: only the outward corner of the start/end cell rounds.
  for (const week of out) {
    const hasStart = week.some(c => c.isStart)
    const hasEnd = week.some(c => c.isEnd)
    for (const cell of week) {
      if (!cell.inRange) continue
      // A lone corner has no sibling radius to share the edge clamp with, so "full" balloons past the caps above — pin it to 50%.
      if (hasStart && hasEnd) cell.roundingClass = cell.isStart && cell.isEnd ? 'rounded-full' : cell.isStart ? 'rounded-l-full' : cell.isEnd ? 'rounded-r-full' : ''
      else if (hasStart && cell.isStart) cell.roundingClass = 'rounded-tl-[50%]'
      else if (hasEnd && cell.isEnd) cell.roundingClass = 'rounded-br-[50%]'
    }
  }
  return out
})

// pick adjusts the existing range in place once both ends are set, instead of forcing the
// two-click pick-start/pick-end flow all over again: clicking before the range moves its
// start back, clicking after extends the end, clicking inside shrinks from the nearer edge.
function pick(cell: DayCell) {
  const d = startOfDay(cell.date)

  if (!rangeStart.value) {
    rangeStart.value = d
    rangeEnd.value = d
    emit('select', d, d)
    return
  }

  if (d < rangeStart.value) {
    rangeStart.value = d
  } else if (d > rangeEnd.value) {
    rangeEnd.value = d
  } else {
    const distToStart = d.getTime() - rangeStart.value.getTime()
    const distToEnd = rangeEnd.value.getTime() - d.getTime()
    if (distToStart <= distToEnd) rangeStart.value = d
    else rangeEnd.value = d
  }
  emit('select', rangeStart.value, rangeEnd.value)
}

function reset() {
  rangeStart.value = null
  rangeEnd.value = null
  emit('reset')
}
</script>

<template>
  <div class="w-72">
    <div class="mb-2 flex items-center justify-between gap-1">
      <button type="button" class="btn btn-ghost btn-xs btn-circle shrink-0" aria-label="Previous month" @click="prevMonth">
        <Icon name="fa6-solid:chevron-left" size="10" />
      </button>
      <div class="flex items-center gap-1">
        <select v-model.number="viewMonthIndex" class="select select-xs">
          <option v-for="(m, i) in monthNames" :key="m" :value="i">
            {{ m }}
          </option>
        </select>
        <input
          v-model="yearInput"
          type="text"
          inputmode="numeric"
          aria-label="Year"
          class="input input-xs w-14 text-center tabular-nums"
          @blur="commitYear"
          @keydown.enter="($event.target as HTMLInputElement).blur()"
        >
      </div>
      <button type="button" class="btn btn-ghost btn-xs btn-circle shrink-0" aria-label="Next month" @click="nextMonth">
        <Icon name="fa6-solid:chevron-right" size="10" />
      </button>
    </div>

    <div class="grid grid-cols-7">
      <span v-for="wd in weekdayLabels" :key="wd" class="text-base-content/40 py-1 text-center text-[10px] font-medium">
        {{ wd }}
      </span>
    </div>

    <div class="grid grid-cols-7">
      <div v-for="week in weeks" :key="week[0]!.date.toISOString()" class="col-span-7 grid grid-cols-7">
        <div v-for="cell in week" :key="cell.date.toISOString()" class="relative aspect-square">
          <div v-if="cell.inRange" class="bg-primary absolute inset-0" :class="cell.roundingClass" />
          <button
            type="button"
            class="relative z-10 flex h-full w-full items-center justify-center rounded-full text-xs transition-colors"
            :class="[
              !cell.inMonth ? 'text-base-content/25' : cell.inRange ? 'text-primary-content font-semibold' : 'text-base-content hover:bg-base-300/50',
              cell.isToday && !cell.inRange ? 'ring-primary/50 ring-1' : '',
            ]"
            @click="pick(cell)"
          >
            {{ cell.date.getDate() }}
          </button>
        </div>
      </div>
    </div>

    <div class="mt-2 flex items-center justify-between gap-2">
      <p class="text-base-content/50 text-[11px]">
        {{ rangeLabel }}
      </p>
      <button v-if="rangeStart" type="button" class="btn btn-ghost btn-xs" @click="reset">
        Reset
      </button>
    </div>
  </div>
</template>
