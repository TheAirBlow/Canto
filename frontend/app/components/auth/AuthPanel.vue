<script setup lang="ts">
import { ApiError } from '~/composables/useApi'

const props = defineProps<{ mode: 'login' | 'register' }>()

const { open, close } = useDrawer()
const auth = useAuthStore()

const username = ref('')
const password = ref('')
const inviteCode = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    if (props.mode === 'login') {
      await auth.login(username.value, password.value)
    } else {
      await auth.register(username.value, password.value, inviteCode.value || undefined)
    }
    close()
    await navigateTo(`/users/${auth.me!.id}`)
  } catch (err) {
    error.value = err instanceof ApiError ? (err.body?.detail ?? err.message) : 'Something went wrong'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <AppModal :title="mode === 'login' ? 'Log in' : 'Create an account'" @close="close">
    <form class="flex flex-col gap-3" @submit.prevent="submit">
      <input v-model="username" type="text" placeholder="Username" class="input w-full" required>
      <input v-model="password" type="password" placeholder="Password" class="input w-full" required>
      <input
        v-if="mode === 'register'"
        v-model="inviteCode"
        type="text"
        placeholder="Invite code (if required)"
        class="input w-full"
      >
      <p v-if="error" class="text-error text-sm">
        {{ error }}
      </p>
      <button type="submit" class="btn btn-primary" :class="{ 'btn-disabled': loading }">
        {{ mode === 'login' ? 'Log in' : 'Register' }}
      </button>
    </form>
    <p class="mt-4 text-sm">
      <template v-if="mode === 'login'">
        No account? <button type="button" class="link" @click="open('register')">Register</button>
      </template>
      <template v-else>
        Already have an account? <button type="button" class="link" @click="open('login')">Log in</button>
      </template>
    </p>
  </AppModal>
</template>
