<script setup lang="ts">
const props = withDefaults(defineProps<{ label?: string, confirmLabel?: string, disabled?: boolean }>(), {
  label: '',
  confirmLabel: 'Confirm?',
})
const emit = defineEmits<{ confirmed: [] }>()

const confirming = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

function click() {
  if (props.disabled) return
  if (confirming.value) {
    clearTimeout(timer)
    confirming.value = false
    emit('confirmed')
    return
  }
  confirming.value = true
  timer = setTimeout(() => {
    confirming.value = false
  }, 3000)
}

onScopeDispose(() => clearTimeout(timer))
</script>

<template>
  <button type="button" :class="{ 'btn-error': confirming }" :disabled="disabled" @click="click">
    <slot :confirming="confirming">
      {{ confirming ? confirmLabel : label }}
    </slot>
  </button>
</template>
