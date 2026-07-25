<script setup lang="ts">
useHead({ title: 'Global' })
</script>

<template>
  <div>
    <h1 class="font-display mb-6 text-3xl font-semibold">
      Instance-wide statistics
    </h1>
    <StatsDashboard scope="global" />
  </div>
</template>
