import { defineStore } from 'pinia'
import { ApiError, useApi } from '~/composables/useApi'
import type { User } from '~/types/api'

export const useAuthStore = defineStore('auth', () => {
  const me = ref<User | null>(null)
  const ready = ref(false)
  const authed = computed(() => me.value !== null)

  async function fetchMe() {
    try {
      me.value = await useApi<User>('/user/me')
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        me.value = null
      } else {
        throw err
      }
    } finally {
      ready.value = true
    }
  }

  async function login(username: string, password: string) {
    me.value = await useApi<User>('/user/login', { method: 'POST', body: { username, password } })
  }

  async function register(username: string, password: string, inviteCode?: string) {
    me.value = await useApi<User>('/user/register', {
      method: 'POST',
      body: { username, password, invite_code: inviteCode },
    })
  }

  async function logout() {
    await useApi('/user/logout', { method: 'POST' })
    me.value = null
  }

  return { me, ready, authed, fetchMe, login, register, logout }
})
