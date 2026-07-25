<script setup lang="ts">
const { toasts, dismiss } = useToast()

const icon: Record<string, string> = {
  success: 'fa6-solid:circle-check',
  error: 'fa6-solid:circle-exclamation',
  info: 'fa6-solid:circle-info',
}
const alertClass: Record<string, string> = {
  success: 'alert-success',
  error: 'alert-error',
  info: 'alert-info',
}
</script>

<template>
  <div class="toast toast-end toast-bottom z-50">
    <TransitionGroup name="toast">
      <div v-for="t in toasts" :key="t.id" class="alert cursor-pointer shadow-lg" :class="alertClass[t.type]" @click="dismiss(t.id)">
        <Icon :name="icon[t.type]!" size="16" />
        <span class="text-sm">{{ t.message }}</span>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition: all 0.2s ease;
}
.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>
