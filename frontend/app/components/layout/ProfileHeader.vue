<script setup lang="ts">
import type { User } from '~/types/api'

const props = defineProps<{ user: User }>()
const auth = useAuthStore()
const { open } = useDrawer()
const isOwn = computed(() => auth.me?.id === props.user.id)

const { imageUrl: ambientImageUrl, active: ambientActive, claimed: ambientClaimed } = useAmbientBackdrop()
watchEffect(() => { ambientImageUrl.value = props.user.image_url })
ambientActive.value = true
ambientClaimed.value = true

const root = ref<HTMLElement>()
useAmbientBackdropHeight(root)
</script>

<template>
  <div ref="root" class="mb-6 pt-8 pb-6 sm:pt-10 sm:pb-8">
    <div class="flex items-center gap-5">
      <div class="avatar">
        <div class="bg-base-300 aspect-square w-20 rounded-full">
          <img v-if="user.image_url" :src="user.image_url" :alt="user.username">
          <div v-else class="flex h-full w-full items-center justify-center">
            <Icon name="fa6-solid:user" size="32" />
          </div>
        </div>
      </div>
      <div class="flex-1">
        <h1 class="font-display flex items-center gap-2 text-3xl font-semibold">
          {{ user.display_name || user.username }}
          <Icon v-if="user.is_admin" name="fa6-solid:shield-halved" size="16" class="text-warning" />
        </h1>
        <p class="text-base-content/60">
          @{{ user.username }} · Member since {{ new Date(user.created_at).toLocaleDateString() }}
        </p>
        <p v-if="user.description" class="mt-2 text-sm">
          {{ user.description }}
        </p>
      </div>
      <button v-if="isOwn" class="btn btn-outline btn-sm" @click="open('settings')">
        <Icon name="fa6-solid:pen" size="12" /> Edit profile
      </button>
    </div>
  </div>
</template>
