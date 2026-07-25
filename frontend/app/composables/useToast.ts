interface Toast {
  id: number
  message: string
  type: 'success' | 'error' | 'info'
}

// Global toast queue backed by useState, so any component can push a transient notification.
export function useToast() {
  const toasts = useState<Toast[]>('toasts', () => [])
  const nextId = useState('toastNextId', () => 0)

  function push(message: string, type: Toast['type'], duration = 3500) {
    const id = nextId.value++
    toasts.value.push({ id, message, type })
    setTimeout(() => {
      const idx = toasts.value.findIndex(t => t.id === id)
      if (idx !== -1) toasts.value.splice(idx, 1)
    }, duration)
  }

  function dismiss(id: number) {
    const idx = toasts.value.findIndex(t => t.id === id)
    if (idx !== -1) toasts.value.splice(idx, 1)
  }

  return {
    toasts,
    dismiss,
    success: (message: string) => push(message, 'success'),
    error: (message: string) => push(message, 'error'),
    info: (message: string) => push(message, 'info'),
  }
}
