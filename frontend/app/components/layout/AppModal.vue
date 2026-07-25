<script setup lang="ts">
withDefaults(defineProps<{ title?: string, maxWidth?: string, height?: string }>(), { title: '', maxWidth: 'max-w-md', height: '' })
const emit = defineEmits<{ close: [] }>()

const dialogRef = useTemplateRef('dialog')
onMounted(() => dialogRef.value?.showModal())
</script>

<template>
  <dialog ref="dialog" class="modal" @close="emit('close')">
    <div class="modal-box flex flex-col" :class="[maxWidth, height || 'max-h-[85vh]']">
      <form method="dialog">
        <button class="btn btn-sm btn-circle btn-ghost absolute top-3 right-3" aria-label="Close">
          <Icon name="fa6-solid:xmark" size="14" />
        </button>
      </form>
      <h3 v-if="title" class="mb-4 shrink-0 text-lg font-bold">
        {{ title }}
      </h3>
      <div class="min-h-0 flex-1 overflow-y-auto">
        <slot />
      </div>
    </div>
    <form method="dialog" class="modal-backdrop">
      <button>close</button>
    </form>
  </dialog>
</template>
