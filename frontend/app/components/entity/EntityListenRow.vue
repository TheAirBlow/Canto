<script setup lang="ts">
import type { EntityListen } from '~/types/api'

defineProps<{ listen: EntityListen }>()
</script>

<template>
  <div class="hover:bg-base-200 flex items-center gap-3 rounded-lg p-2 text-sm transition-colors">
    <Icon name="fa6-solid:play" size="12" class="text-base-content/40" />
    <span v-if="listen.user">
      <NuxtLink v-if="listen.user.id" :to="`/users/${listen.user.id}`" class="font-medium hover:underline">
        {{ listen.user.display_name || listen.user.username }}
      </NuxtLink>
      <span v-else class="font-medium">Private user</span>
    </span>
    <span v-else class="text-base-content/50">Someone</span>
    <span class="text-base-content/50 ml-auto">{{ new Date(listen.listened_at).toLocaleString() }}</span>
  </div>
</template>
