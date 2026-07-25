<script setup lang="ts">
import type { useTimeframe } from '~/composables/useTimeframe'

const props = defineProps<{ timeframe: ReturnType<typeof useTimeframe> }>()

const currentYear = new Date().getFullYear()

const presets = [
  { label: 'Today', apply: () => props.timeframe.setPreset('day') },
  { label: 'This week', apply: () => props.timeframe.setPreset('week') },
  { label: 'This month', apply: () => props.timeframe.setPreset('month') },
  { label: 'This year', apply: () => props.timeframe.setPreset('year') },
  { label: 'Last year', apply: () => props.timeframe.setYear(currentYear - 1) },
  { label: 'All time', apply: () => props.timeframe.setPreset('all_time') },
]

const activeLabel = computed(() => {
  const tf = props.timeframe
  if (tf.from.value !== undefined) return 'Custom'
  if (tf.year.value === currentYear - 1) return 'Last year'
  switch (tf.period.value) {
    case 'day': return 'Today'
    case 'week': return 'This week'
    case 'month': return 'This month'
    case 'year': return 'This year'
    default: return 'All time'
  }
})

// restore replays whichever preset was active before Custom, so Reset lands back there instead of always "All time".
const restore = ref(presets.find(p => p.label === activeLabel.value)?.apply ?? presets[presets.length - 1]!.apply)

function selectPreset(p: typeof presets[number]) {
  restore.value = p.apply
  p.apply()
}

const customOpen = ref(false)
const popoverRef = useTemplateRef('popover')
onClickOutside(popoverRef, () => (customOpen.value = false))

function applyCustom(from: Date, to: Date) {
  const inclusiveTo = new Date(to)
  inclusiveTo.setDate(inclusiveTo.getDate() + 1) // Timeframe.Resolve's `to` bound is exclusive
  props.timeframe.setCustomRange(from, inclusiveTo)
}

function resetCustom() {
  restore.value()
}
</script>

<template>
  <div class="flex flex-wrap items-center gap-1.5">
    <button
      v-for="p in presets"
      :key="p.label"
      class="btn btn-sm rounded-full"
      :class="activeLabel === p.label ? 'btn-primary' : 'btn-ghost'"
      @click="selectPreset(p)"
    >
      {{ p.label }}
    </button>
    <div ref="popover" class="relative">
      <button
        class="btn btn-sm rounded-full"
        :class="activeLabel === 'Custom' ? 'btn-primary' : 'btn-ghost'"
        @click="customOpen = !customOpen"
      >
        <Icon name="fa6-solid:calendar-days" size="12" /> Custom
      </button>
      <Transition name="pop">
        <div
          v-if="customOpen"
          class="bg-base-200 rounded-box border-base-300 absolute top-full right-0 z-20 mt-2 max-w-[calc(100vw-2rem)] border p-3 shadow-lg"
        >
          <DateRangeCalendar
            :initial-from="timeframe.from.value ? new Date(timeframe.from.value * 1000) : undefined"
            :initial-to="timeframe.to.value ? new Date((timeframe.to.value - 86400) * 1000) : undefined"
            @select="applyCustom"
            @reset="resetCustom"
          />
        </div>
      </Transition>
    </div>
  </div>
</template>
