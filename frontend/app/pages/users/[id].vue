<script setup lang="ts">
import type { User } from '~/types/api'

const route = useRoute()
const user = await useApi<User>(`/user/${route.params.id}`)

useHead({ title: user.display_name || user.username })
</script>

<template>
  <div>
    <ProfileHeader :user="user" />
    <StatsDashboard :scope="String(user.id)" />
  </div>
</template>
