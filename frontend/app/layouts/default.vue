<script setup lang="ts">
const { imageUrl: ambientImageUrl, active: ambientActive, claimed: ambientClaimed, height: ambientHeight } = useAmbientBackdrop()
const nuxtApp = useNuxtApp()
nuxtApp.hook('page:start', () => { ambientClaimed.value = false })
nuxtApp.hook('page:finish', () => {
  if (ambientClaimed.value) return
  ambientImageUrl.value = undefined
  ambientActive.value = false
  ambientHeight.value = 0
})
</script>

<template>
  <div class="bg-base-100 text-base-content relative isolate min-h-screen sm:pl-18">
    <div class="absolute inset-x-0 top-0 -z-10 overflow-hidden transition-[height] duration-300 ease-out" :style="{ height: `${ambientHeight}px` }">
      <AmbientBackdrop />
    </div>

    <AppNavRail />

    <main class="mx-auto max-w-[1600px] px-4 py-6">
      <slot />
    </main>

    <DrawerHost />
    <ToastHost />
  </div>
</template>
