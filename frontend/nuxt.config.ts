import tailwindcss from '@tailwindcss/vite'

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  devtools: { enabled: true },

  app: {
    head: {
      titleTemplate: '%s · Canto',
      title: 'Canto',
      meta: [{ name: 'description', content: 'A self-hosted music listen tracker.' }],
    },
    pageTransition: { name: 'fade', mode: 'out-in' },
  },

  modules: [
    '@pinia/nuxt',
    'pinia-plugin-persistedstate/nuxt',
    '@nuxtjs/color-mode',
    '@vueuse/nuxt',
    '@nuxt/icon',
    '@nuxt/eslint',
    'nuxt-charts',
  ],

  components: [{ path: '~/components', pathPrefix: false }],

  css: ['~/assets/css/main.css'],
  vite: { plugins: [tailwindcss()] },

  experimental: { asyncContext: true },

  colorMode: {
    classSuffix: '',
    dataValue: 'theme',
    preference: 'dracula',
    fallback: 'dracula',
  },

  icon: {
    clientBundle: {
      scan: true,
      sizeLimitKb: 512,
    },
  },

  runtimeConfig: {
    apiBase: 'http://127.0.0.1:3000/api',
    public: {
      apiBase: '/api',
    },
  },
})
