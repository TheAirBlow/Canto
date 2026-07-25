// How far past the header's own height the backdrop extends.
export const AMBIENT_BACKDROP_OVERSHOOT = 128

// Shared hero image URL so the layout-level background gradient can react to whatever page header is mounted.
export function useAmbientBackdrop() {
  const imageUrl = useState<string | undefined>('ambientBackdropImage', () => undefined)
  const active = useState('ambientBackdropActive', () => false)
  const claimed = useState('ambientBackdropClaimed', () => false)
  const height = useState('ambientBackdropHeight', () => 0)
  return { imageUrl, active, claimed, height }
}

export function useAmbientBackdropHeight(root: Ref<HTMLElement | undefined>) {
  const { height } = useAmbientBackdrop()
  useResizeObserver(root, () => {
    const el = root.value!
    if (el.offsetHeight === 0) return
    height.value = el.offsetTop + el.offsetHeight + AMBIENT_BACKDROP_OVERSHOOT
  })
}
