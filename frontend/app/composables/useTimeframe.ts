export interface TimeframeQuery {
  period?: string
  year?: number
  month?: number
  week?: number
  from?: number
  to?: number
  tz?: string
}

// Shared reactive period/year/month/week/from/to model, matching Canto's shared stats timeframe query params.
export function useTimeframe(defaultPeriod = 'all_time') {
  const preferences = usePreferencesStore()
  const period = ref(preferences.lastPeriod || defaultPeriod)
  const year = ref<number>()
  const month = ref<number>()
  const week = ref<number>()
  const from = ref<number>()
  const to = ref<number>()

  // setCustomRange picks the explicit from/to path (clearing period/year/month/week, which Timeframe.Resolve prefers).
  function setCustomRange(fromDate: Date, toDate: Date) {
    period.value = ''
    year.value = undefined
    month.value = undefined
    week.value = undefined
    from.value = Math.floor(fromDate.getTime() / 1000)
    to.value = Math.floor(toDate.getTime() / 1000)
  }

  // setPreset picks a rolling/calendar period, clearing any custom range, and remembers it for next visit.
  function setPreset(preset: string) {
    period.value = preset
    year.value = undefined
    month.value = undefined
    week.value = undefined
    from.value = undefined
    to.value = undefined
    preferences.lastPeriod = preset
  }

  // setYear pins a specific calendar year (e.g. "last year"), bypassing period.
  function setYear(y: number) {
    period.value = ''
    year.value = y
    month.value = undefined
    week.value = undefined
    from.value = undefined
    to.value = undefined
  }

  function toQuery(): TimeframeQuery {
    return {
      period: period.value || undefined,
      year: year.value,
      month: month.value,
      week: week.value,
      from: from.value,
      to: to.value,
      tz: Intl.DateTimeFormat().resolvedOptions().timeZone,
    }
  }

  return { period, year, month, week, from, to, setCustomRange, setPreset, setYear, toQuery }
}
