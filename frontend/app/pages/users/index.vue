<script setup lang="ts">
import type { User } from '~/types/api'

useHead({ title: 'Users' })

const { items, done, loading, error, loadMore } = useInfiniteList<User>({
  kind: 'cursor',
  limit: 50,
  fetchPage: async (after, limit) => {
    const users = await useApi<User[]>('/user', { query: { after, limit } })
    const nextAfter = users.length > 0 ? users[users.length - 1]!.id : undefined
    return { items: users, nextAfter }
  },
})

await loadMore()
const shown = useDelayedPending(loading)
</script>

<template>
  <div>
    <h1 class="font-display mb-6 text-3xl font-semibold">
      Users
    </h1>

    <Transition name="fade" mode="out-in">
      <div v-if="items.length === 0 && shown" key="skeleton" class="grid gap-3 sm:grid-cols-2">
        <div v-for="i in 6" :key="i" class="skeleton h-20 w-full" />
      </div>

      <p v-else-if="error" key="error" class="text-error">
        Failed to load users.
        <button class="link" @click="loadMore">
          Retry
        </button>
      </p>

      <p v-else-if="items.length === 0" key="empty" class="text-base-content/60">
        No public profiles yet.
      </p>

      <div v-else key="content" class="grid gap-3 sm:grid-cols-2">
        <UserCard v-for="user in items" :key="user.id" :user="user" />
      </div>
    </Transition>

    <InfiniteSentinel :disabled="done || loading" @load="loadMore" />
  </div>
</template>
