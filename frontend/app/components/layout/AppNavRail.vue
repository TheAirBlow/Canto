<script setup lang="ts">
const auth = useAuthStore()
const { open } = useDrawer()

function onKeydown(event: KeyboardEvent) {
  const target = event.target as HTMLElement
  const typing = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable
  if (event.key === '/' && !typing) {
    event.preventDefault()
    open('search')
  }
}

onMounted(() => window.addEventListener('keydown', onKeydown))
onUnmounted(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div
    class="bg-base-200 border-base-300 z-40 flex items-center justify-between border-b px-3 py-2
      sm:fixed sm:top-0 sm:left-0 sm:h-screen sm:w-18 sm:flex-col sm:justify-between sm:border-r sm:border-b-0 sm:px-0 sm:py-6"
  >
    <div class="flex items-center gap-1 sm:flex-col sm:gap-2">
      <NuxtLink to="/" class="btn btn-ghost btn-circle tooltip tooltip-bottom sm:tooltip-right" data-tip="Home">
        <Icon name="fa6-solid:record-vinyl" size="20" />
      </NuxtLink>
      <NuxtLink to="/global" class="btn btn-ghost btn-circle tooltip tooltip-bottom sm:tooltip-right" data-tip="Global">
        <Icon name="fa6-solid:globe" size="18" />
      </NuxtLink>
      <NuxtLink to="/users" class="btn btn-ghost btn-circle tooltip tooltip-bottom sm:tooltip-right" data-tip="Users">
        <Icon name="fa6-solid:users" size="18" />
      </NuxtLink>
      <NuxtLink v-if="auth.authed" to="/rewind" class="btn btn-ghost btn-circle tooltip tooltip-bottom sm:tooltip-right" data-tip="Rewind">
        <Icon name="fa6-solid:clock-rotate-left" size="18" />
      </NuxtLink>
    </div>

    <div class="flex items-center gap-1 sm:flex-col sm:gap-2">
      <button class="btn btn-ghost btn-circle tooltip tooltip-bottom sm:tooltip-right" data-tip="Search" aria-label="Search (press /)" @click="open('search')">
        <Icon name="fa6-solid:magnifying-glass" size="18" />
      </button>
      <button class="btn btn-ghost btn-circle tooltip tooltip-bottom sm:tooltip-right" data-tip="Settings" @click="open('settings')">
        <Icon name="fa6-solid:gear" size="18" />
      </button>
      <template v-if="auth.authed">
        <NuxtLink :to="`/users/${auth.me?.id}`" class="btn btn-ghost btn-circle tooltip tooltip-bottom sm:tooltip-right" data-tip="My profile">
          <Icon name="fa6-solid:circle-user" size="20" />
        </NuxtLink>
      </template>
      <template v-else>
        <button class="btn btn-ghost btn-circle tooltip tooltip-bottom sm:tooltip-right" data-tip="Log in" @click="open('login')">
          <Icon name="fa6-solid:right-to-bracket" size="18" />
        </button>
        <button class="btn btn-primary btn-circle tooltip tooltip-bottom sm:tooltip-right" data-tip="Register" @click="open('register')">
          <Icon name="fa6-solid:user-plus" size="16" />
        </button>
      </template>
    </div>
  </div>
</template>
