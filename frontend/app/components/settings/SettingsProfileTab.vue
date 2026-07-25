<script setup lang="ts">
const auth = useAuthStore()
const { close } = useDrawer()

const { loading: loggingOut, run: logout } = useAsyncAction(async () => {
  await auth.logout()
  close()
  await navigateTo('/')
})

const displayName = ref(auth.me?.display_name ?? '')
const description = ref(auth.me?.description ?? '')
const isPublic = ref(auth.me?.public ?? true)
const fileInput = useTemplateRef('fileInput')

const { loading: saving, run: save } = useAsyncAction(async () => {
  auth.me = await useApi('/user/me', {
    method: 'PUT',
    body: { display_name: displayName.value || undefined, description: description.value || undefined, public: isPublic.value },
  })
}, 'Profile saved.')

const { loading: uploading, run: onFileChange } = useAsyncAction(async (event: Event) => {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  const form = new FormData()
  form.append('image', file)
  await useApi('/user/me/image', { method: 'PUT', body: form })
  await auth.fetchMe()
}, 'Photo updated.')

const toast = useToast()
const currentPassword = ref('')
const newUsername = ref('')
const newPassword = ref('')
const confirmPassword = ref('')

const { loading: savingCredentials, run: saveCredentials } = useAsyncAction(async () => {
  if (!currentPassword.value) {
    toast.error('Enter your current password.')
    return
  }
  if (!newUsername.value.trim() && !newPassword.value) {
    toast.error('Enter a new username or password.')
    return
  }
  if (newPassword.value && newPassword.value !== confirmPassword.value) {
    toast.error("New passwords don't match.")
    return
  }
  auth.me = await useApi('/user/me/credentials', {
    method: 'PUT',
    body: {
      current_password: currentPassword.value,
      new_username: newUsername.value.trim() || undefined,
      new_password: newPassword.value || undefined,
    },
  })
  currentPassword.value = ''
  newUsername.value = ''
  newPassword.value = ''
  confirmPassword.value = ''
}, 'Credentials updated.')
</script>

<template>
  <div class="flex flex-col gap-4">
    <div class="flex items-center gap-4">
      <div class="avatar">
        <div class="bg-base-300 w-16 rounded-full">
          <img v-if="auth.me?.image_url" :src="auth.me.image_url" :alt="auth.me?.username">
          <div v-else class="flex h-full w-full items-center justify-center">
            <Icon name="fa6-solid:user" size="24" />
          </div>
        </div>
      </div>
      <button class="btn btn-sm" :class="{ 'btn-disabled': uploading }" @click="fileInput?.click()">
        <span v-if="uploading" class="loading loading-spinner loading-xs" /> Change photo
      </button>
      <input ref="fileInput" type="file" accept="image/*" class="hidden" @change="onFileChange">
    </div>

    <fieldset class="fieldset">
      <legend class="fieldset-legend">Display name</legend>
      <input v-model="displayName" type="text" class="input w-full">
    </fieldset>
    <fieldset class="fieldset">
      <legend class="fieldset-legend">Bio</legend>
      <textarea v-model="description" class="textarea w-full" rows="3" />
    </fieldset>
    <label class="label cursor-pointer justify-start gap-3">
      <input v-model="isPublic" type="checkbox" class="toggle toggle-primary">
      <span>Public profile</span>
    </label>

    <button class="btn btn-primary" :class="{ 'btn-disabled': saving }" @click="save">
      <span v-if="saving" class="loading loading-spinner loading-xs" /> Save
    </button>

    <div class="divider">
      Username &amp; password
    </div>
    <fieldset class="fieldset">
      <legend class="fieldset-legend">New username</legend>
      <input v-model="newUsername" type="text" :placeholder="auth.me?.username" class="input w-full">
    </fieldset>
    <fieldset class="fieldset">
      <legend class="fieldset-legend">New password</legend>
      <input v-model="newPassword" type="password" placeholder="Leave blank to keep current password" class="input w-full">
    </fieldset>
    <fieldset v-if="newPassword" class="fieldset">
      <legend class="fieldset-legend">Confirm new password</legend>
      <input v-model="confirmPassword" type="password" class="input w-full">
    </fieldset>
    <p v-if="newPassword" class="text-warning text-xs">
      Changing your password signs out every other session — you'll stay logged in here, but other devices will need to log in again.
    </p>
    <fieldset class="fieldset">
      <legend class="fieldset-legend">Current password</legend>
      <input v-model="currentPassword" type="password" placeholder="Required to confirm changes" class="input w-full">
    </fieldset>
    <button class="btn btn-sm w-fit" :class="{ 'btn-disabled': savingCredentials }" @click="saveCredentials">
      <span v-if="savingCredentials" class="loading loading-spinner loading-xs" /> Update credentials
    </button>

    <div class="divider" />
    <button class="btn btn-outline btn-error" :class="{ 'btn-disabled': loggingOut }" @click="logout">
      <span v-if="loggingOut" class="loading loading-spinner loading-xs" /> <Icon v-else name="fa6-solid:right-from-bracket" size="12" /> Log out
    </button>
  </div>
</template>
