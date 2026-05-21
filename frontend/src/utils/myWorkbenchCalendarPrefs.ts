import type { Dayjs } from 'dayjs'

/** 0 = 周日，1 = 周一 */
export type WeekStartsOn = 0 | 1

export const MY_WORKBENCH_CALENDAR_WEEK_START_KEY = 'zb.myworkbench.calendar.weekStartsOn'

const WEEKDAY_LABELS_SUN = ['日', '一', '二', '三', '四', '五', '六'] as const

export function readWeekStartsOn(): WeekStartsOn {
  try {
    const v = localStorage.getItem(MY_WORKBENCH_CALENDAR_WEEK_START_KEY)
    if (v === '1') return 1
    return 0
  } catch {
    return 0
  }
}

export function saveWeekStartsOn(value: WeekStartsOn): void {
  try {
    localStorage.setItem(MY_WORKBENCH_CALENDAR_WEEK_START_KEY, String(value))
  } catch {
    /* ignore quota / private mode */
  }
}

export function getWeekdayLabels(weekStartsOn: WeekStartsOn): string[] {
  if (weekStartsOn === 0) return [...WEEKDAY_LABELS_SUN]
  return [...WEEKDAY_LABELS_SUN.slice(1), WEEKDAY_LABELS_SUN[0]]
}

/** 含 d 所在周的第一天（按 weekStartsOn） */
export function startOfWeek(d: Dayjs, weekStartsOn: WeekStartsOn): Dayjs {
  const dow = d.day()
  const diff = (dow - weekStartsOn + 7) % 7
  return d.subtract(diff, 'day').startOf('day')
}

/** 含 d 所在周的最后一天 */
export function endOfWeek(d: Dayjs, weekStartsOn: WeekStartsOn): Dayjs {
  return startOfWeek(d, weekStartsOn).add(6, 'day').startOf('day')
}
