<script setup lang="ts">
const props = defineProps<{
  kind: 'artist' | 'album' | 'song'
  id: number
  name: string
  imageUrl?: string
}>()

const href = computed(() => `/${props.kind}s/${props.id}`)
const fallbackIcon = computed(() => ({ artist: 'fa6-solid:users', album: 'fa6-solid:compact-disc', song: 'fa6-solid:music' })[props.kind])
</script>

<template>
  <NuxtLink :to="href" class="group/card flex flex-col gap-2">
    <div
      class="bg-base-300 aspect-square w-full overflow-hidden transition-transform group-hover/card:scale-105"
      :class="kind === 'artist' ? 'rounded-full' : 'rounded-lg'"
    >
      <img v-if="imageUrl" :src="imageUrl" :alt="name" class="h-full w-full object-cover">
      <div v-else class="flex h-full w-full items-center justify-center">
        <Icon :name="fallbackIcon" size="32" class="text-base-content/30" />
      </div>
    </div>
    <p class="truncate text-sm font-medium">
      {{ name }}
    </p>
  </NuxtLink>
</template>
