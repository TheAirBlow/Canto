export type PanelName = 'login' | 'register' | 'settings' | 'search'

// Query-param-driven panel state so drawers/modals are deep-linkable and render correctly on SSR.
export function useDrawer() {
  const route = useRoute()
  const router = useRouter()

  const current = computed(() => route.query.panel as PanelName | undefined)

  function open(panel: PanelName, params: Record<string, string> = {}) {
    router.push({ query: { ...route.query, panel, ...params } })
  }

  function close() {
    const rest = { ...route.query }
    delete rest.panel
    router.push({ query: rest })
  }

  return { current, open, close }
}
