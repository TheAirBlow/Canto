interface PagePagination<T> {
  kind: 'page'
  // total is omitted by endpoints like GET /stats/{scope}/top/{kind} that take page/per_page but
  // return a bare array; a short page is then the only end-of-list signal.
  fetchPage: (page: number, perPage: number) => Promise<{ items: T[], total?: number }>
  perPage?: number
}

interface CursorPagination<T> {
  kind: 'cursor'
  fetchPage: (after: number | undefined, limit: number) => Promise<{ items: T[], nextAfter: number | undefined }>
  limit?: number
}

// Drives infinite scroll off either of Canto's two pagination shapes: page/per_page/total
// (most list endpoints) or after/limit cursor (GET /user, which has no total).
export function useInfiniteList<T>(strategy: PagePagination<T> | CursorPagination<T>) {
  const items = ref<T[]>([]) as Ref<T[]>
  const done = ref(false)
  const loading = ref(false)
  const error = ref<unknown>(null)

  let page = 1
  let after: number | undefined

  async function loadMore() {
    if (done.value || loading.value) return
    loading.value = true
    error.value = null
    try {
      if (strategy.kind === 'page') {
        const perPage = strategy.perPage ?? 20
        const { items: newItems, total } = await strategy.fetchPage(page, perPage)
        items.value.push(...newItems)
        page += 1
        if (newItems.length === 0) done.value = true
        else if (total !== undefined) done.value = items.value.length >= total
        else done.value = newItems.length < perPage
      } else {
        const limit = strategy.limit ?? 50
        const { items: newItems, nextAfter } = await strategy.fetchPage(after, limit)
        items.value.push(...newItems)
        after = nextAfter
        if (nextAfter === undefined || newItems.length < limit) done.value = true
      }
    } catch (err) {
      error.value = err
    } finally {
      loading.value = false
    }
  }

  function reset() {
    items.value = []
    done.value = false
    loading.value = false
    error.value = null
    page = 1
    after = undefined
  }

  return { items, done, loading, error, loadMore, reset }
}
