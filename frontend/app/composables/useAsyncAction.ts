import { ApiError } from '~/composables/useApi'

// Wraps a mutating action with loading state and toast feedback, so a backend failure always
// shows something instead of an uncaught rejection propagating through Vue's event-handler
// error pipeline. Pass successMessage to toast a confirmation once fn resolves.
export function useAsyncAction<Args extends unknown[]>(fn: (...args: Args) => Promise<void>, successMessage?: string) {
  const loading = ref(false)
  const toast = useToast()

  async function run(...args: Args) {
    loading.value = true
    try {
      await fn(...args)
      if (successMessage) toast.success(successMessage)
    } catch (err) {
      toast.error(err instanceof ApiError ? (err.body?.detail ?? err.message) : 'Something went wrong.')
    } finally {
      loading.value = false
    }
  }

  return { loading, run }
}
