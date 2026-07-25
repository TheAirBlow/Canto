<script setup lang="ts">
const { imageUrl, active } = useAmbientBackdrop()
const tint = useAverageColor(imageUrl)
</script>

<template>
  <div class="pointer-events-none absolute inset-0">
    <Transition name="fade">
      <img
        v-if="imageUrl"
        :key="imageUrl"
        :src="imageUrl"
        alt=""
        class="absolute inset-0 h-full w-full scale-125 object-cover opacity-25 blur-3xl"
      >
    </Transition>
    <!--<Transition name="fade">
      <div
        v-if="tint"
        :key="tint"
        class="absolute inset-0"
        :style="{ background: `linear-gradient(to bottom, ${tint.replace(')', ' / 0.45)')}, transparent 85%)` }"
      />
    </Transition>-->
    <Transition name="fade">
      <div v-if="active && !imageUrl" class="bg-base-300 absolute inset-0" />
    </Transition>
    <div class="from-base-100 absolute inset-x-0 bottom-0 h-1/3 bg-gradient-to-t to-transparent" />
  </div>
</template>
