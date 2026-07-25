<script setup lang="ts">
import type { NuxtError } from '#app'

const props = defineProps<{ error: NuxtError }>()

const message = computed(() => {
  if (props.error.statusCode === 404) return 'This page doesn\'t exist.'
  if (props.error.statusCode === 403) return 'You don\'t have access to this.'
  return props.error.statusMessage || 'Something went wrong.'
})
</script>

<template>
  <div class="bg-base-100 text-base-content flex min-h-screen flex-col items-center justify-center gap-4 px-4 text-center">
    <Icon name="fa6-solid:record-vinyl" size="40" class="text-base-content/30" />
    <p class="text-base-content/50 text-6xl font-bold">
      {{ error.statusCode }}
    </p>
    <p class="text-base-content/70">
      {{ message }}
    </p>
    <NuxtLink to="/" class="btn btn-primary btn-sm" @click="clearError({ redirect: '/' })">
      Take me home
    </NuxtLink>
  </div>
</template>
