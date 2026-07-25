<script setup lang="ts">
const route = useRoute()
const { close } = useDrawer()
const auth = useAuthStore()

const tabs = computed(() => {
  if (!auth.authed) return ['Appearance']
  const base = ['Profile', 'Appearance', 'API Keys', 'Ingest', 'Blacklist', 'Import', 'Export']
  if (auth.me?.is_admin) base.push('Invites', 'Catalog', 'Maintenance')
  return base
})

const active = ref((route.query.tab as string) ?? (auth.authed ? 'Profile' : 'Appearance'))
</script>

<template>
  <AppModal title="Settings" max-width="max-w-5xl" height="h-[90vh] sm:h-[85vh]" @close="close">
    <div class="flex h-full flex-col gap-6 sm:flex-row">
      <div class="flex shrink-0 gap-1 overflow-x-auto sm:h-full sm:w-48 sm:flex-col sm:overflow-x-visible sm:overflow-y-auto">
        <button
          v-for="tab in tabs"
          :key="tab"
          type="button"
          class="hover:bg-base-200 rounded-box shrink-0 px-3 py-2 text-left text-sm whitespace-nowrap"
          :class="active === tab ? 'bg-base-200 text-primary font-semibold' : 'text-base-content/70'"
          @click="active = tab"
        >
          {{ tab }}
        </button>
      </div>

      <div class="min-h-0 min-w-0 flex-1 overflow-y-auto sm:h-full">
        <Transition name="fade" mode="out-in">
          <SettingsProfileTab v-if="active === 'Profile'" key="Profile" />
          <SettingsAppearanceTab v-else-if="active === 'Appearance'" key="Appearance" />
          <SettingsApiKeysTab v-else-if="active === 'API Keys'" key="API Keys" />
          <SettingsIngestTab v-else-if="active === 'Ingest'" key="Ingest" />
          <SettingsBlacklistTab v-else-if="active === 'Blacklist'" key="Blacklist" />
          <SettingsImportTab v-else-if="active === 'Import'" key="Import" />
          <SettingsExportTab v-else-if="active === 'Export'" key="Export" />
          <AdminInvitesTab v-else-if="active === 'Invites'" key="Invites" />
          <AdminCatalogTab v-else-if="active === 'Catalog'" key="Catalog" />
          <AdminMaintenanceTab v-else-if="active === 'Maintenance'" key="Maintenance" />
        </Transition>
      </div>
    </div>
  </AppModal>
</template>
