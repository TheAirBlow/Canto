<script setup lang="ts">
import type { User } from '~/types/api'

defineProps<{ user: User }>()
</script>

<template>
  <NuxtLink :to="`/users/${user.id}`" class="card bg-base-200 hover:bg-base-300 transition-colors">
    <div class="card-body flex-row items-center gap-4 p-4">
      <div class="avatar placeholder">
        <div class="bg-neutral text-neutral-content w-12 rounded-full flex items-center justify-center">
          <img v-if="user.image_url" :src="user.image_url" :alt="user.username">
          <Icon v-else name="fa6-solid:user" size="20" />
        </div>
      </div>
      <div>
        <p class="font-semibold">
          {{ user.display_name || user.username }}
        </p>
        <p class="text-base-content/60 text-sm">
          @{{ user.username }}
        </p>
      </div>
      <Icon v-if="user.is_admin" name="fa6-solid:shield-halved" size="14" class="text-warning ml-auto" />
    </div>
  </NuxtLink>
</template>
