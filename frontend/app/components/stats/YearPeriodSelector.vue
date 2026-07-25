<script setup lang="ts">
const year = defineModel<number | null>('year', { required: true })

const currentYear = new Date().getUTCFullYear()
const years = Array.from({ length: 6 }, (_, i) => currentYear - i)

function prev() {
  year.value = (year.value ?? currentYear) - 1
}
function next() {
  const n = (year.value ?? currentYear) + 1
  year.value = n > currentYear ? null : n
}
</script>

<template>
  <div class="flex items-center gap-1">
    <button class="btn btn-ghost btn-xs btn-circle" aria-label="Previous year" @click="prev">
      <Icon name="fa6-solid:chevron-left" size="12" />
    </button>
    <select
      class="select select-bordered select-xs w-32"
      :value="year ?? 'rolling'"
      @change="year = ($event.target as HTMLSelectElement).value === 'rolling' ? null : Number(($event.target as HTMLSelectElement).value)"
    >
      <option value="rolling">
        Last 12 months
      </option>
      <option v-for="y in years" :key="y" :value="y">
        {{ y }}
      </option>
    </select>
    <button class="btn btn-ghost btn-xs btn-circle" aria-label="Next year" @click="next">
      <Icon name="fa6-solid:chevron-right" size="12" />
    </button>
  </div>
</template>
