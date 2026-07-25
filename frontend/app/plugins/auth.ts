export default defineNuxtPlugin(async () => {
  const auth = useAuthStore()
  if (!auth.ready) {
    await auth.fetchMe()
  }
})
