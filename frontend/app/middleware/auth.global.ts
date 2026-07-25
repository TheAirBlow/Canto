export default defineNuxtRouteMiddleware((to) => {
  if (to.path !== '/') return

  const auth = useAuthStore()
  return navigateTo(auth.authed ? `/users/${auth.me!.id}` : '/users')
})
