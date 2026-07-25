<script setup lang="ts">
import type { EntityListeningNow } from '~/types/api'

defineProps<{ entries: EntityListeningNow[] }>()
</script>

<template>
  <div v-if="entries.length > 0" class="flex flex-wrap gap-2">
    <NuxtLink
      v-for="(entry, i) in entries"
      :key="i"
      :to="entry.user.id ? `/users/${entry.user.id}` : ''"
      class="badge badge-success gap-1.5"
    >
      <span class="relative flex h-1.5 w-1.5">
        <span class="bg-success-content absolute inline-flex h-full w-full animate-ping rounded-full opacity-75" />
        <span class="bg-success-content relative inline-flex h-1.5 w-1.5 rounded-full" />
      </span>
      {{ entry.user.display_name || entry.user.username || 'Someone' }}
    </NuxtLink>
  </div>
</template>
