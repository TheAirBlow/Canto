<script setup lang="ts">
import { storeToRefs } from 'pinia'
import type { CustomThemeColors } from '~/stores/theme'

const themes = [
  'light', 'dark', 'cupcake', 'bumblebee', 'emerald', 'corporate', 'synthwave', 'retro', 'cyberpunk', 'valentine',
  'halloween', 'garden', 'forest', 'aqua', 'lofi', 'pastel', 'fantasy', 'wireframe', 'black', 'luxury', 'dracula',
  'cmyk', 'autumn', 'business', 'acid', 'lemonade', 'night', 'coffee', 'winter', 'dim', 'nord', 'sunset',
  'caramellatte', 'abyss', 'silk',
]

const sampleCustomTheme: CustomThemeColors = {
  base100: '#181825',
  base200: '#1e1e2e',
  base300: '#313244',
  baseContent: '#cdd6f4',
  primary: '#cba6f7',
  primaryContent: '#11111b',
  secondary: '#f5c2e7',
  secondaryContent: '#11111b',
  accent: '#94e2d5',
  accentContent: '#11111b',
  neutral: '#313244',
  neutralContent: '#cdd6f4',
  info: '#89b4fa',
  infoContent: '#11111b',
  success: '#a6e3a1',
  successContent: '#11111b',
  warning: '#f9e2af',
  warningContent: '#11111b',
  error: '#f38ba8',
  errorContent: '#11111b',
}

const colorMode = useColorMode()
const themeStore = useThemeStore()
const { customActive, customColors } = storeToRefs(themeStore)

const customJson = ref(JSON.stringify(customColors.value ?? sampleCustomTheme, null, 2))
const customError = ref('')
const toast = useToast()

function selectTheme(theme: string) {
  colorMode.preference = theme
  if (customActive.value) themeStore.deactivate()
}

function selectCustom() {
  try {
    themeStore.activate(JSON.parse(customJson.value))
  } catch {
    customActive.value = true
  }
}

function applyCustom() {
  customError.value = ''
  try {
    const parsed = JSON.parse(customJson.value) as CustomThemeColors
    themeStore.activate(parsed)
    toast.success('Custom theme applied.')
  } catch (err) {
    customError.value = err instanceof Error ? err.message : 'Invalid JSON.'
  }
}
</script>

<template>
  <div>
    <p class="text-base-content/60 mb-4 text-xs">
      Pick a theme — applies immediately, remembered on this device.
    </p>
    <div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
      <button
        v-for="theme in themes"
        :key="theme"
        type="button"
        :data-theme="theme"
        class="bg-base-100 rounded-box outline-base-300 flex cursor-pointer flex-col gap-2.5 p-3 text-left outline transition-transform hover:-translate-y-0.5"
        :class="!customActive && colorMode.preference === theme ? 'outline-primary outline-2' : 'outline-1'"
        @click="selectTheme(theme)"
      >
        <span class="text-base-content text-xs font-medium capitalize">{{ theme }}</span>
        <span class="flex gap-1">
          <span class="bg-primary h-5 w-5 rounded-full" />
          <span class="bg-secondary h-5 w-5 rounded-full" />
          <span class="bg-accent h-5 w-5 rounded-full" />
          <span class="bg-neutral h-5 w-5 rounded-full" />
        </span>
      </button>

      <button
        type="button"
        class="bg-base-100 rounded-box outline-base-300 flex cursor-pointer flex-col gap-2.5 p-3 text-left outline transition-transform hover:-translate-y-0.5"
        :class="customActive ? 'outline-primary outline-2' : 'outline-1 border-dashed'"
        @click="selectCustom"
      >
        <span class="text-base-content flex items-center gap-1.5 text-xs font-medium">
          <Icon name="fa6-solid:palette" size="10" /> Custom
        </span>
        <span class="text-base-content/50 text-[11px] leading-tight">
          Your own colors
        </span>
      </button>
    </div>

    <Transition name="fade">
      <div v-if="customActive" class="mt-6">
        <h4 class="mb-1 text-sm font-semibold">
          Custom theme
        </h4>
        <p class="text-base-content/60 mb-3 text-xs">
          Hand-pick exact colors as hex values. They layer on top of whichever theme was active — leave any key out
          to fall back to that theme's own color.
        </p>
        <textarea
          v-model="customJson"
          spellcheck="false"
          class="textarea bg-base-100 h-72 w-full font-mono text-xs"
        />
        <p v-if="customError" class="text-error mt-1 text-xs">
          {{ customError }}
        </p>
        <button class="btn btn-sm btn-primary mt-2" @click="applyCustom">
          Apply
        </button>
      </div>
    </Transition>
  </div>
</template>
