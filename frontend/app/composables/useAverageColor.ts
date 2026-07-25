// A flat 1x1 average tends to land pale/muddy, so push saturation and lightness toward a visible mid-range.
function vividize(r: number, g: number, b: number): string {
  const rn = r / 255
  const gn = g / 255
  const bn = b / 255
  const max = Math.max(rn, gn, bn)
  const min = Math.min(rn, gn, bn)
  const d = max - min
  const l = (max + min) / 2

  let h = 0
  if (d !== 0) {
    switch (max) {
      case rn: h = ((gn - bn) / d) % 6; break
      case gn: h = (bn - rn) / d + 2; break
      default: h = (rn - gn) / d + 4; break
    }
    h *= 60
    if (h < 0) h += 360
  }
  const s = Math.min(1, (d === 0 ? 0 : d / (1 - Math.abs(2 * l - 1))) * 1.6 + 0.15)
  const boostedL = Math.min(0.62, Math.max(0.38, l))
  return `hsl(${h.toFixed(1)} ${(s * 100).toFixed(0)}% ${(boostedL * 100).toFixed(0)}%)`
}

// Samples an image's average color (via a 1x1 canvas downscale) for ambient tinting; resolves to null on failure.
export function useAverageColor(src: Ref<string | undefined> | ComputedRef<string | undefined>) {
  const color = ref<string | null>(null)

  watch(src, (url) => {
    color.value = null
    if (!url || !import.meta.client) return

    const img = new Image()
    img.onload = () => {
      try {
        const canvas = document.createElement('canvas')
        canvas.width = 1
        canvas.height = 1
        const ctx = canvas.getContext('2d')
        if (!ctx) return
        ctx.drawImage(img, 0, 0, 1, 1)
        const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data
        color.value = vividize(r!, g!, b!)
      } catch {
        // Cross-origin canvas read failure — leave untinted.
      }
    }
    img.src = url
  }, { immediate: true })

  return color
}
