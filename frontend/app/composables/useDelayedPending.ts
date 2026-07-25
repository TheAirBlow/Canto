// Gates a loading flag behind a show delay and a minimum visible duration, so fast fetches never flash a skeleton.
export function useDelayedPending(pending: Ref<boolean> | ComputedRef<boolean>, showDelay = 150, minVisible = 300) {
  const shown = ref(false)
  let showTimer: ReturnType<typeof setTimeout> | undefined
  let hideTimer: ReturnType<typeof setTimeout> | undefined
  let shownAt = 0

  watch(pending, (isPending) => {
    clearTimeout(showTimer)
    clearTimeout(hideTimer)
    if (isPending) {
      showTimer = setTimeout(() => {
        shown.value = true
        shownAt = Date.now()
      }, showDelay)
    } else if (shown.value) {
      const remaining = minVisible - (Date.now() - shownAt)
      if (remaining > 0) hideTimer = setTimeout(() => { shown.value = false }, remaining)
      else shown.value = false
    }
  }, { immediate: true })

  onScopeDispose(() => {
    clearTimeout(showTimer)
    clearTimeout(hideTimer)
  })

  return shown
}
