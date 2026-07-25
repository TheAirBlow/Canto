<script setup lang="ts">
defineProps<{ scope: string }>()
const emit = defineEmits<{ 'update:scope': [string] }>()
const auth = useAuthStore()
</script>

<template>
  <div v-if="auth.authed" class="join">
    <button
      class="btn btn-sm join-item"
      :class="scope === 'global' ? 'btn-primary' : 'btn-ghost'"
      @click="emit('update:scope', 'global')"
    >
      Global
    </button>
    <button
      class="btn btn-sm join-item"
      :class="scope !== 'global' ? 'btn-primary' : 'btn-ghost'"
      @click="emit('update:scope', String(auth.me!.id))"
    >
      Mine
    </button>
  </div>
</template>
