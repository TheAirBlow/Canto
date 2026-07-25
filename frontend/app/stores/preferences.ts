import { defineStore } from 'pinia'

// Small settings the UI remembers across visits (last period picked, heatmap/graph step+range) — not app data.
export const usePreferencesStore = defineStore('preferences', () => {
  const lastPeriod = ref('all_time')
  const discoveryStep = ref('week')
  const interestStep = ref('week')

  return { lastPeriod, discoveryStep, interestStep }
}, {
  persist: {
    storage: piniaPluginPersistedstate.localStorage(),
  },
})
