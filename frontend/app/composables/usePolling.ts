// Polls fetchOnce every intervalMs until isTerminal(result) is true, or the caller stops it.
export function usePolling<T>(fetchOnce: () => Promise<T>, isTerminal: (value: T) => boolean, intervalMs = 1500) {
  const data = ref<T | null>(null) as Ref<T | null>
  let timer: ReturnType<typeof setInterval> | undefined

  async function tick() {
    try {
      const result = await fetchOnce()
      data.value = result
      if (isTerminal(result)) stop()
    } catch {
      // transient poll failure - next tick retries, caller keeps showing the last known state
    }
  }

  function stop() {
    if (timer) clearInterval(timer)
    timer = undefined
  }

  tick()
  timer = setInterval(tick, intervalMs)
  onScopeDispose(stop)

  return { data, stop }
}
