<script setup lang="ts">
defineProps<{ label: string, value: string | number, icon?: string, sub?: string }>()
</script>

<template>
  <div class="bg-base-200 rounded-box border-base-300 border flex flex-col gap-1.5 p-5">
    <div class="text-base-content/60 flex items-center gap-2 text-xs font-medium tracking-wide uppercase">
      <Icon v-if="icon" :name="icon" size="14" />
      {{ label }}
    </div>
    <div class="font-display text-3xl font-semibold tracking-tight">
      {{ value }}
    </div>
    <div v-if="sub" class="text-base-content/50 text-xs">
      {{ sub }}
    </div>
  </div>
</template>
