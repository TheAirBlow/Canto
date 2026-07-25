<script setup lang="ts">
const props = defineProps<{ name: string, imageUrl?: string, description?: string, pinned?: boolean, typeLabel: string }>()

const isArtist = computed(() => props.typeLabel === 'Artist')

const { imageUrl: ambientImageUrl, active: ambientActive, claimed: ambientClaimed } = useAmbientBackdrop()
watchEffect(() => { ambientImageUrl.value = props.imageUrl })
ambientActive.value = true
ambientClaimed.value = true

const root = ref<HTMLElement>()
useAmbientBackdropHeight(root)
</script>

<template>
  <div ref="root" class="mb-8 pt-8 pb-6 sm:pt-14 sm:pb-10">
    <div class="flex flex-col gap-5 sm:flex-row sm:items-end">
      <div class="avatar shrink-0">
        <div class="bg-base-300 aspect-square w-32 sm:w-56" :class="isArtist ? 'rounded-full' : 'rounded-lg'">
          <img v-if="imageUrl" :src="imageUrl" :alt="name">
          <div v-else class="flex h-full w-full items-center justify-center">
            <Icon name="fa6-solid:music" size="40" class="text-base-content/30" />
          </div>
        </div>
      </div>
      <div class="min-w-0 flex-1">
        <p class="text-base-content/60 flex items-center gap-2 text-xs font-semibold uppercase">
          {{ typeLabel }}
          <Icon v-if="pinned" name="fa6-solid:thumbtack" size="10" />
        </p>
        <h1
          class="font-display font-semibold tracking-tight"
          :class="name.length > 28 ? 'text-2xl sm:text-4xl lg:text-5xl' : 'text-3xl sm:text-5xl lg:text-6xl'"
        >
          {{ name }}
        </h1>
        <div class="mt-2">
          <slot name="sub" />
        </div>
        <p v-if="description" class="mt-2 max-w-2xl text-sm">
          {{ description }}
        </p>
        <div class="mt-4">
          <slot name="meta" />
        </div>
      </div>
    </div>
  </div>
</template>
