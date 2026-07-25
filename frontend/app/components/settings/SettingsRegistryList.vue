<script setup lang="ts">
const props = defineProps<{ modelValue: string[], available: { id: string, available: boolean }[] }>()
const emit = defineEmits<{ 'update:modelValue': [string[]] }>()

const pending = ref('')

const candidates = computed(() => props.available.filter(a => !props.modelValue.includes(a.id)))

function labelFor(id: string) {
  return props.available.find(a => a.id === id)?.available ?? true
}

// optionsFor lists what slot idx can be set to: everything not already used in another slot, plus its own current value.
function optionsFor(idx: number) {
  const usedElsewhere = new Set(props.modelValue.filter((_, i) => i !== idx))
  return props.available.filter(a => !usedElsewhere.has(a.id))
}

function replace(idx: number, id: string) {
  const v = [...props.modelValue]
  v[idx] = id
  emit('update:modelValue', v)
}

function add() {
  if (!pending.value) return
  emit('update:modelValue', [...props.modelValue, pending.value])
  pending.value = ''
}

function remove(idx: number) {
  const v = [...props.modelValue]
  v.splice(idx, 1)
  emit('update:modelValue', v)
}

function move(idx: number, dir: -1 | 1) {
  const v = [...props.modelValue]
  const swapWith = idx + dir
  if (swapWith < 0 || swapWith >= v.length) return
  ;[v[idx], v[swapWith]] = [v[swapWith]!, v[idx]!]
  emit('update:modelValue', v)
}
</script>

<template>
  <div class="flex flex-col gap-1">
    <div v-for="(id, idx) in modelValue" :key="id" class="join w-full">
      <IconSelect
        class="join-item capitalize"
        :class="{ 'select-warning': !labelFor(id) }"
        :model-value="id"
        :options="optionsFor(idx).map(opt => ({ value: opt.id, label: serviceMeta(opt.id).label + (opt.available ? '' : ' (unavailable)'), icon: serviceMeta(opt.id).icon, disabled: !opt.available }))"
        @update:model-value="v => replace(idx, v)"
      />
      <button class="btn btn-ghost join-item px-2" :disabled="idx === 0" @click="move(idx, -1)">
        <Icon name="fa6-solid:chevron-up" size="10" />
      </button>
      <button class="btn btn-ghost join-item px-2" :disabled="idx === modelValue.length - 1" @click="move(idx, 1)">
        <Icon name="fa6-solid:chevron-down" size="10" />
      </button>
      <button class="btn btn-ghost join-item text-error px-2" @click="remove(idx)">
        <Icon name="fa6-solid:trash" size="10" />
      </button>
    </div>

    <div v-if="candidates.length > 0" class="join mt-1 w-full">
      <IconSelect
        v-model="pending"
        class="join-item"
        placeholder="Add a processor"
        :options="candidates.map(c => ({ value: c.id, label: serviceMeta(c.id).label + (c.available ? '' : ' (unavailable)'), icon: serviceMeta(c.id).icon, disabled: !c.available }))"
      />
      <button class="btn btn-sm join-item" :disabled="!pending" @click="add">
        <Icon name="fa6-solid:plus" size="10" /> Add
      </button>
    </div>
  </div>
</template>
