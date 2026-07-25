import { proxyRequest } from 'h3'

// Same-origin proxy for every /api/** request — required since Canto's cookie auth has no CORS support.
export default defineEventHandler((event) => {
  const config = useRuntimeConfig()
  const path = event.path.slice('/api'.length)
  return proxyRequest(event, `${config.apiBase}${path}`)
})
