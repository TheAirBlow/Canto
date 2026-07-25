<script setup lang="ts">
defineProps<{ kind: 'artist' | 'album' | 'song', id: number }>()

const auth = useAuthStore()
const showEdit = ref(false)
</script>

<template>
  <div v-if="auth.me?.is_admin">
    <button class="btn btn-outline btn-sm" @click="showEdit = !showEdit">
      <Icon name="fa6-solid:pen" size="12" /> {{ showEdit ? 'Close' : 'Manage' }}
    </button>
    <div v-if="showEdit" class="bg-base-200 rounded-box border-base-300 border mt-3 p-4">
      <EntityEditDrawer :id="id" :kind="kind" @updated="showEdit = false" @deleted="navigateTo('/')" />
    </div>
  </div>
</template>
