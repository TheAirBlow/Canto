// Buckets values into 5 quantile-based intensity levels (0 = zero, 1-4 = increasing), shared by
// ActivityHeatmap and ClockHeatmap so both agree on the same color breakpoints.
export function useHeatmapScale(values: Ref<number[]> | ComputedRef<number[]>) {
  const breakpoints = computed(() => {
    const nonZero = [...toValue(values)].filter(v => v > 0).sort((a, b) => a - b)
    if (nonZero.length === 0) return [0, 0, 0, 0]
    const quantile = (p: number) => nonZero[Math.min(nonZero.length - 1, Math.floor(p * nonZero.length))]!
    return [quantile(0.25), quantile(0.5), quantile(0.75), quantile(1)]
  })

  function level(value: number): 0 | 1 | 2 | 3 | 4 {
    if (value <= 0) return 0
    const [q1, q2, q3] = breakpoints.value
    if (value <= q1!) return 1
    if (value <= q2!) return 2
    if (value <= q3!) return 3
    return 4
  }

  return { level }
}
