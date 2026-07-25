<script setup lang="ts">
const props = withDefaults(defineProps<{
  kind: 'artist' | 'album' | 'song'
  id: number
  name: string
  imageUrl?: string
  sub?: string
  subParts?: { text: string, href?: string }[]
  trailing?: string | number
  trailingSub?: string
  rank?: number
  blacklisted?: boolean
  delta?: number
  size?: 'sm' | 'md'
  linkable?: boolean
}>(), {
  imageUrl: undefined, sub: undefined, subParts: undefined, trailing: undefined, trailingSub: undefined,
  rank: undefined, delta: undefined, size: 'sm', linkable: true,
})

const href = computed(() => `/${props.kind}s/${props.id}`)
const fallbackIcon = computed(() => ({ artist: 'fa6-solid:users', album: 'fa6-solid:compact-disc', song: 'fa6-solid:music' })[props.kind])
const avatarSize = computed(() => ({ sm: 'w-10', md: 'w-12' })[props.size])
</script>

<template>
  <div
    class="hover:bg-base-200 relative flex items-center gap-3 rounded-lg p-2 transition-colors"
    :class="{ 'opacity-50': blacklisted, 'cursor-pointer': !linkable }"
  >
    <span v-if="rank !== undefined" class="text-base-content/40 w-6 shrink-0 text-right font-mono text-sm">{{ rank }}</span>
    <div class="avatar">
      <div class="bg-base-300" :class="[avatarSize, kind === 'artist' ? 'rounded-full' : 'rounded']">
        <img v-if="imageUrl" :src="imageUrl" :alt="name">
        <div v-else class="flex h-full w-full items-center justify-center">
          <Icon :name="fallbackIcon" size="16" />
        </div>
      </div>
    </div>
    <div class="min-w-0 flex-1">
      <p class="flex items-center gap-1.5 font-medium">
        <NuxtLink v-if="linkable" :to="href" class="relative z-10 min-w-0 truncate hover:underline">
          {{ name }}
          <span class="absolute inset-0" />
        </NuxtLink>
        <span v-else class="min-w-0 truncate">{{ name }}</span>
        <span v-if="blacklisted" class="badge badge-ghost badge-xs shrink-0 gap-1 normal-case">
          <Icon name="fa6-solid:ban" size="8" /> Blacklisted
        </span>
      </p>
      <p v-if="subParts?.length" class="text-base-content/60 truncate text-xs">
        <template v-for="(part, i) in subParts" :key="i">
          <span v-if="i > 0"> · </span>
          <NuxtLink v-if="linkable && part.href" :to="part.href" class="relative z-10 hover:underline">{{ part.text }}</NuxtLink>
          <span v-else>{{ part.text }}</span>
        </template>
      </p>
      <p v-else-if="sub" class="text-base-content/60 truncate text-xs">
        {{ sub }}
      </p>
    </div>
    <span
      v-if="delta !== undefined && delta !== 0"
      class="flex shrink-0 items-center gap-0.5 text-xs font-medium"
      :class="delta > 0 ? 'text-success' : 'text-error'"
    >
      <Icon :name="delta > 0 ? 'fa6-solid:caret-up' : 'fa6-solid:caret-down'" size="10" /> {{ Math.abs(delta) }}
    </span>
    <span v-if="trailing !== undefined" class="flex shrink-0 flex-col items-end">
      <span class="text-base-content/60 text-sm">{{ trailing }}</span>
      <span v-if="trailingSub" class="text-base-content/40 text-xs">{{ trailingSub }}</span>
    </span>
  </div>
</template>
