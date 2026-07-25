import { defineStore } from 'pinia'

export interface CustomThemeColors {
  base100?: string
  base200?: string
  base300?: string
  baseContent?: string
  primary?: string
  primaryContent?: string
  secondary?: string
  secondaryContent?: string
  accent?: string
  accentContent?: string
  neutral?: string
  neutralContent?: string
  info?: string
  infoContent?: string
  success?: string
  successContent?: string
  warning?: string
  warningContent?: string
  error?: string
  errorContent?: string
}

// cssVarByKey maps each CustomThemeColors key to the daisyUI CSS custom property it overrides.
const cssVarByKey: Record<keyof CustomThemeColors, string> = {
  base100: '--color-base-100',
  base200: '--color-base-200',
  base300: '--color-base-300',
  baseContent: '--color-base-content',
  primary: '--color-primary',
  primaryContent: '--color-primary-content',
  secondary: '--color-secondary',
  secondaryContent: '--color-secondary-content',
  accent: '--color-accent',
  accentContent: '--color-accent-content',
  neutral: '--color-neutral',
  neutralContent: '--color-neutral-content',
  info: '--color-info',
  infoContent: '--color-info-content',
  success: '--color-success',
  successContent: '--color-success-content',
  warning: '--color-warning',
  warningContent: '--color-warning-content',
  error: '--color-error',
  errorContent: '--color-error-content',
}

// Layers hand-picked colors on top of the active daisyUI theme via inline styles on <html>, which win the cascade.
// customColors and customActive are tracked separately, so picking a built-in theme can turn the override off
// without losing the user's draft palette.
export const useThemeStore = defineStore('theme', () => {
  const customColors = ref<CustomThemeColors | null>(null)
  const customActive = ref(false)

  function applyToDom() {
    if (import.meta.server) return
    for (const [key, cssVar] of Object.entries(cssVarByKey)) {
      const value = customActive.value ? customColors.value?.[key as keyof CustomThemeColors] : undefined
      if (value) document.documentElement.style.setProperty(cssVar, value)
      else document.documentElement.style.removeProperty(cssVar)
    }
  }

  function activate(colors: CustomThemeColors) {
    customColors.value = colors
    customActive.value = true
    applyToDom()
  }

  function deactivate() {
    customActive.value = false
    applyToDom()
  }

  return { customColors, customActive, activate, deactivate, applyToDom }
}, {
  persist: {
    storage: piniaPluginPersistedstate.localStorage(),
    pick: ['customColors', 'customActive'],
  },
})
