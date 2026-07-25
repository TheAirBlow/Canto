<script setup lang="ts">
const { current } = useDrawer()
</script>

<template>
  <AuthPanel v-if="current === 'login' || current === 'register'" :mode="current" />
  <SearchPanel v-else-if="current === 'search'" />
  <SettingsPanel v-else-if="current === 'settings'" />
</template>
