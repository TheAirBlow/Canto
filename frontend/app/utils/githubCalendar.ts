import type { ActivityBucket } from '~/types/api'

export interface CalendarCell {
  date: string
  count: number
  minutes: number
  inRange: boolean
}

export type CalendarWeek = CalendarCell[]

const monthNames = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

// yearBounds returns [from, to] in UTC for year (a specific calendar year) or null (the rolling last ~53 weeks), matching how the backend buckets activity.
export function yearBounds(year: number | null, now = new Date()): { from: Date, to: Date } {
  if (year === null) {
    const to = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()))
    const from = new Date(to)
    from.setUTCDate(from.getUTCDate() - 371)
    return { from, to }
  }
  return { from: new Date(Date.UTC(year, 0, 1)), to: new Date(Date.UTC(year, 11, 31)) }
}

// calendarWeeks buckets buckets into Monday-aligned UTC week columns spanning [from, to], padding cells outside that range as out-of-range.
export function calendarWeeks(buckets: ActivityBucket[], from: Date, to: Date): CalendarWeek[] {
  const byDate = new Map(buckets.map(b => [b.bucket.slice(0, 10), b]))
  const start = new Date(from)
  start.setUTCDate(start.getUTCDate() - ((start.getUTCDay() + 6) % 7))

  const weeks: CalendarWeek[] = []
  const cur = new Date(start)
  while (cur <= to) {
    const week: CalendarCell[] = []
    for (let i = 0; i < 7; i++) {
      const iso = cur.toISOString().slice(0, 10)
      const bucket = byDate.get(iso)
      week.push({ date: iso, count: bucket?.listen_count ?? 0, minutes: bucket?.minutes_listened ?? 0, inRange: cur >= from && cur <= to })
      cur.setUTCDate(cur.getUTCDate() + 1)
    }
    weeks.push(week)
  }
  return weeks
}

// calendarRangeCaption formats weeks' in-range span as e.g. "Jan 2025 – Jul 2026", or "" if nothing is in range.
export function calendarRangeCaption(weeks: CalendarWeek[]): string {
  const inRange = weeks.flat().filter(c => c.inRange)
  if (inRange.length === 0) return ''
  const a = new Date(inRange[0]!.date)
  const b = new Date(inRange[inRange.length - 1]!.date)
  const fa = `${monthNames[a.getUTCMonth()]} ${a.getUTCFullYear()}`
  const fb = `${monthNames[b.getUTCMonth()]} ${b.getUTCFullYear()}`
  return fa === fb ? fa : `${fa} – ${fb}`
}
