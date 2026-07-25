// Artist/album/song detail routes.
const entityPath = /^\/(artists|albums|songs)\/[^/]+$/

// Defaults an entity page's ?scope= from where the visitor navigated from: a user's own
// profile page scopes to that user, anything else (including /global) scopes to global.
export default defineNuxtRouteMiddleware((to, from) => {
  if (!entityPath.test(to.path) || to.query.scope) return

  const fromUser = from.path.match(/^\/users\/(\d+)/)
  const scope = fromUser ? fromUser[1]! : 'global'
  return navigateTo({ path: to.path, query: { ...to.query, scope } }, { replace: true })
})
