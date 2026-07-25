<script setup lang="ts" generic="T extends string">
interface IconSelectOption<T> {
  value: T
  label: string
  icon?: string
  disabled?: boolean
}

defineOptions({ inheritAttrs: false })
const props = defineProps<{ modelValue: T, options: IconSelectOption<T>[], placeholder?: string, size?: 'sm' | 'md' }>()
const emit = defineEmits<{ 'update:modelValue': [T] }>()

const open = ref(false)
const rootRef = useTemplateRef('root')
onClickOutside(rootRef, () => (open.value = false))

const selected = computed(() => props.options.find(o => o.value === props.modelValue))

function pick(opt: IconSelectOption<T>) {
  if (opt.disabled) return
  emit('update:modelValue', opt.value)
  open.value = false
}
</script>

<template>
  <div ref="root" class="relative flex-1">
    <button
      type="button"
      class="select w-full items-center justify-start gap-2"
      :class="size === 'md' ? '' : 'select-sm'"
      v-bind="$attrs"
      @click="open = !open"
    >
      <Icon v-if="selected?.icon" :name="selected.icon" size="12" />
      <span class="truncate">{{ selected?.label ?? placeholder ?? 'Select…' }}</span>
    </button>
    <Transition name="pop">
      <div v-if="open" class="bg-base-200 border-base-300 rounded-box absolute z-20 mt-1 max-h-60 w-full overflow-y-auto border py-1 shadow-lg">
        <button
          v-for="opt in options"
          :key="opt.value"
          type="button"
          class="hover:bg-base-300 flex w-full items-center gap-2 px-2 py-1.5 text-left text-sm"
          :class="[opt.disabled ? 'text-base-content/40 cursor-not-allowed' : '', opt.value === modelValue ? 'bg-base-300' : '']"
          @click="pick(opt)"
        >
          <Icon v-if="opt.icon" :name="opt.icon" size="12" />
          <span class="truncate">{{ opt.label }}</span>
        </button>
      </div>
    </Transition>
  </div>
</template>
