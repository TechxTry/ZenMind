import React, { useEffect, useMemo, useState } from 'react'
import { Button, Modal, Segmented, Space, Switch, Typography } from 'antd'
import type { Dayjs } from 'dayjs'
import dayjs from 'dayjs'
import { Link } from 'react-router-dom'
import type { CalendarExternalEvent } from '../api'
import {
  endOfWeek,
  getWeekdayLabels,
  readWeekStartsOn,
  startOfWeek,
  type WeekStartsOn,
} from '../utils/myWorkbenchCalendarPrefs'

const { Text } = Typography

type ViewMode = 'month' | 'list'

export const CALENDAR_CATEGORY_COLORS = {
  effort: '#1677ff',
  taskPlan: '#52c41a',
  external: '#fa8c16',
} as const

/** 个人工作台月历：是否在格子里展示每日报工工时 */
export const MY_WORKBENCH_CALENDAR_SHOW_HOURS_KEY = 'zb.myworkbench.calendar.showHours'

function readShowDailyHours(): boolean {
  try {
    const v = localStorage.getItem(MY_WORKBENCH_CALENDAR_SHOW_HOURS_KEY)
    if (v === null) return true
    return v !== 'false'
  } catch {
    return true
  }
}

export function getCalendarEventCategory(event: CalendarExternalEvent): 'taskPlan' | 'external' {
  return event.source_type === 'task' ? 'taskPlan' : 'external'
}

export function getCalendarEventDisplayColor(event: CalendarExternalEvent): string {
  return CALENDAR_CATEGORY_COLORS[getCalendarEventCategory(event)]
}

type NormalizedEvent = CalendarExternalEvent & {
  _startDay: Dayjs
  _endDay: Dayjs
  _key: string
}

function normalizeEvent(e: CalendarExternalEvent): NormalizedEvent | null {
  const s = dayjs(e.start)
  const endRaw = dayjs(e.end)
  if (!s.isValid() || !endRaw.isValid()) return null

  // iCal all-day 常见语义：end 为次日 00:00（exclusive）
  let end = endRaw
  if (e.all_day && endRaw.hour() === 0 && endRaw.minute() === 0 && endRaw.second() === 0) {
    end = endRaw.subtract(1, 'day')
  }

  const startDay = s.startOf('day')
  const endDay = end.startOf('day')
  if (endDay.isBefore(startDay)) return null

  const key = `${e.source_type}:${e.source_id}:${e.title}:${e.start}:${e.end}`
  return { ...e, _startDay: startDay, _endDay: endDay, _key: key }
}

function clampDay(d: Dayjs, lo: Dayjs, hi: Dayjs) {
  if (d.isBefore(lo, 'day')) return lo
  if (d.isAfter(hi, 'day')) return hi
  return d
}

function daySpanInclusive(a: Dayjs, b: Dayjs) {
  return b.startOf('day').diff(a.startOf('day'), 'day') + 1
}

type WeekSeg = {
  key: string
  ev: NormalizedEvent
  lane: number
  startIdx: number // 0..6 within week
  span: number // 1..7
  isStart: boolean
  isEnd: boolean
}

const MAX_EVENT_LANES = 3
const EVENT_ROW_STEP = 20
const EVENT_BAR_HEIGHT = 16
const CELL_HEADER_TOP = 8
const CELL_HEADER_HEIGHT = 24
const CELL_OVERLAY_TOP = CELL_HEADER_TOP + CELL_HEADER_HEIGHT + 2
const CELL_MIN_HEIGHT = CELL_OVERLAY_TOP + (MAX_EVENT_LANES + 1) * EVENT_ROW_STEP + 8
const CELL_HEATMAP_MIN_HEIGHT = 76

type Rgba = { r: number; g: number; b: number; a: number }
type ResolvedTheme = 'light' | 'dark'

type EffortHeatPalette = {
  stops: Array<{ t: number; c: Rgba }>
  overload: Rgba
  /** 单元格底色 RGB，用于估算对比度 */
  baseRgb: number
}

/** 深色主题：蓝色系 */
const EFFORT_HEAT_PALETTE_DARK: EffortHeatPalette = {
  stops: [
    { t: 0, c: { r: 255, g: 255, b: 255, a: 0.04 } },
    { t: 0.18, c: { r: 105, g: 177, b: 255, a: 0.2 } },
    { t: 0.38, c: { r: 56, g: 136, b: 255, a: 0.36 } },
    { t: 0.58, c: { r: 22, g: 119, b: 255, a: 0.52 } },
    { t: 0.78, c: { r: 9, g: 88, b: 217, a: 0.66 } },
    { t: 1, c: { r: 0, g: 52, b: 163, a: 0.82 } },
  ],
  overload: { r: 207, g: 88, b: 19, a: 0.86 },
  baseRgb: 18,
}

/** 浅色主题：橙色系 */
const EFFORT_HEAT_PALETTE_LIGHT: EffortHeatPalette = {
  stops: [
    { t: 0, c: { r: 255, g: 255, b: 255, a: 0 } },
    { t: 0.18, c: { r: 255, g: 237, b: 213, a: 0.55 } },
    { t: 0.38, c: { r: 253, g: 186, b: 116, a: 0.68 } },
    { t: 0.58, c: { r: 251, g: 146, b: 60, a: 0.78 } },
    { t: 0.78, c: { r: 249, g: 115, b: 22, a: 0.86 } },
    { t: 1, c: { r: 234, g: 88, b: 12, a: 0.92 } },
  ],
  overload: { r: 220, g: 38, b: 38, a: 0.92 },
  baseRgb: 248,
}

function getEffortHeatPalette(theme: ResolvedTheme): EffortHeatPalette {
  return theme === 'dark' ? EFFORT_HEAT_PALETTE_DARK : EFFORT_HEAT_PALETTE_LIGHT
}

function lerpRgba(a: Rgba, b: Rgba, t: number): Rgba {
  return {
    r: Math.round(a.r + (b.r - a.r) * t),
    g: Math.round(a.g + (b.g - a.g) * t),
    b: Math.round(a.b + (b.b - a.b) * t),
    a: a.a + (b.a - a.a) * t,
  }
}

function sampleEffortHeatColor(intensity: number, palette: EffortHeatPalette): Rgba {
  const t = Math.max(0, Math.min(1, intensity))
  const { stops } = palette
  for (let i = 0; i < stops.length - 1; i++) {
    const lo = stops[i]
    const hi = stops[i + 1]
    if (t >= lo.t && t <= hi.t) {
      const local = hi.t === lo.t ? 1 : (t - lo.t) / (hi.t - lo.t)
      return lerpRgba(lo.c, hi.c, local)
    }
  }
  return stops[stops.length - 1].c
}

/** 叠在单元格底色上的近似亮度，用于挑选日期/工时文字色 */
function compositeLuminanceOnBase(c: Rgba, base: number): number {
  const R = c.r * c.a + base * (1 - c.a)
  const G = c.g * c.a + base * (1 - c.a)
  const B = c.b * c.a + base * (1 - c.a)
  return 0.299 * R + 0.587 * G + 0.114 * B
}

export function effortHeatmapStyle(hours: number, maxHours: number, theme: ResolvedTheme = 'dark') {
  const palette = getEffortHeatPalette(theme)
  const max = maxHours > 0 ? maxHours : 8
  const ratio = Math.max(0, hours) / max
  const intensity = Math.pow(Math.min(1, ratio), 0.72)
  const rgba =
    ratio > 1
      ? lerpRgba(sampleEffortHeatColor(1, palette), palette.overload, Math.min(1, (ratio - 1) / 0.5))
      : sampleEffortHeatColor(intensity, palette)
  const lum = compositeLuminanceOnBase(rgba, palette.baseRgb)
  const lightText = theme === 'dark' ? lum < 145 : lum < 175
  return {
    background: `rgba(${rgba.r},${rgba.g},${rgba.b},${rgba.a})`,
    textColor: lightText ? '#f8fafc' : 'var(--zm-text-primary)',
    dateBadgeBg: lightText ? 'rgba(0,0,0,0.38)' : 'rgba(255,255,255,0.78)',
    dateBadgeBorder: lightText ? 'rgba(255,255,255,0.14)' : 'rgba(15,23,42,0.1)',
    hoursMuted: lightText ? 'rgba(255,255,255,0.82)' : 'var(--zm-text-muted)',
  }
}

function effortHeatmapGradientCss(maxHours: number, theme: ResolvedTheme): string {
  const max = maxHours > 0 ? maxHours : 8
  const samples = [0, 0.25, 0.5, 0.75, 1].map((f) => {
    const { background } = effortHeatmapStyle(f * max, max, theme)
    return background
  })
  return `linear-gradient(90deg, ${samples.join(', ')})`
}

function readDocumentTheme(): ResolvedTheme {
  return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light'
}

function useDocumentTheme(): ResolvedTheme {
  const [resolvedTheme, setResolvedTheme] = useState<ResolvedTheme>(readDocumentTheme)
  useEffect(() => {
    const el = document.documentElement
    const sync = () => setResolvedTheme(readDocumentTheme())
    const obs = new MutationObserver(sync)
    obs.observe(el, { attributes: true, attributeFilter: ['data-theme'] })
    return () => obs.disconnect()
  }, [])
  return resolvedTheme
}

function packWeekSegments(weekStart: Dayjs, events: NormalizedEvent[], maxLanes: number) {
  const weekEnd = weekStart.add(6, 'day')
  const intersects = events
    .filter((e) => !e._endDay.isBefore(weekStart, 'day') && !e._startDay.isAfter(weekEnd, 'day'))
    .sort((a, b) => {
      const ds = a._startDay.diff(b._startDay, 'day')
      if (ds !== 0) return ds
      const de = b._endDay.diff(a._endDay, 'day')
      if (de !== 0) return de
      return a.title.localeCompare(b.title)
    })

  const lanesEnd: Dayjs[] = []
  const segs: WeekSeg[] = []

  for (const ev of intersects) {
    const segStart = clampDay(ev._startDay, weekStart, weekEnd)
    const segEnd = clampDay(ev._endDay, weekStart, weekEnd)
    const startIdx = segStart.diff(weekStart, 'day')
    const span = daySpanInclusive(segStart, segEnd)

    let lane = -1
    for (let i = 0; i < lanesEnd.length; i++) {
      if (lanesEnd[i].isBefore(segStart, 'day')) {
        lane = i
        break
      }
    }
    if (lane === -1) {
      lane = lanesEnd.length
      lanesEnd.push(segEnd)
    } else {
      lanesEnd[lane] = segEnd
    }

    if (lane < maxLanes) {
      segs.push({
        key: `${ev._key}:${weekStart.format('YYYY-MM-DD')}`,
        ev,
        lane,
        startIdx,
        span,
        isStart: ev._startDay.isSame(segStart, 'day'),
        isEnd: ev._endDay.isSame(segEnd, 'day'),
      })
    }
  }

  const hidden = segs.length < intersects.length ? intersects.length - segs.length : 0
  return { segs, hidden }
}

export const MacMonthCalendar: React.FC<{
  month: Dayjs
  selectedDay: Dayjs
  events: CalendarExternalEvent[]
  loading?: boolean
  getCellDots?: (d: Dayjs) => { colors: string[]; n?: number }
  /** 当日禅道报工合计工时（小时） */
  getDailyHours?: (d: Dayjs) => number
  /** 热力图满格参照（通常为每日标准工时，默认 8） */
  heatmapMaxHours?: number
  /** 禅道任务计划（source_type=task）跳转任务详情页 */
  getTaskDetailHref?: (taskId: number) => string
  /** 每周第一天：0 周日，1 周一；未传则从本地偏好读取 */
  weekStartsOn?: WeekStartsOn
  onMonthChange: (d: Dayjs) => void
  onSelectDay: (d: Dayjs) => void
}> = ({
  month,
  selectedDay,
  events,
  loading,
  getCellDots,
  getDailyHours,
  heatmapMaxHours = 8,
  getTaskDetailHref,
  weekStartsOn: weekStartsOnProp,
  onMonthChange,
  onSelectDay,
}) => {
  const [mode, setMode] = useState<ViewMode>('month')
  const [openEv, setOpenEv] = useState<NormalizedEvent | null>(null)
  const [showDailyHours, setShowDailyHours] = useState(readShowDailyHours)
  const weekStartsOn = weekStartsOnProp ?? readWeekStartsOn()

  const normalized = useMemo(() => {
    const out: NormalizedEvent[] = []
    for (const e of events ?? []) {
      const ne = normalizeEvent(e)
      if (ne) out.push(ne)
    }
    return out
  }, [events])

  const grid = useMemo(() => {
    const monthStart = month.startOf('month')
    const monthEnd = month.endOf('month')
    const start = startOfWeek(monthStart, weekStartsOn)
    const end = endOfWeek(monthEnd, weekStartsOn)
    const days: Dayjs[] = []
    for (let d = start; !d.isAfter(end, 'day'); d = d.add(1, 'day')) days.push(d)
    const weeks: Dayjs[][] = []
    for (let i = 0; i < days.length; i += 7) weeks.push(days.slice(i, i + 7))
    return { start, weeks }
  }, [month, weekStartsOn])

  const listByDay = useMemo(() => {
    const mStart = month.startOf('month')
    const mEnd = month.endOf('month')
    const rows = normalized
      .filter((e) => !e._endDay.isBefore(mStart, 'day') && !e._startDay.isAfter(mEnd, 'day'))
      .sort((a, b) => {
        const ds = a._startDay.diff(b._startDay, 'day')
        if (ds !== 0) return ds
        const de = a._endDay.diff(b._endDay, 'day')
        if (de !== 0) return de
        return a.title.localeCompare(b.title)
      })
    return rows
  }, [normalized, month])

  const weekdayLabels = getWeekdayLabels(weekStartsOn)
  const today = dayjs()
  const resolvedTheme = useDocumentTheme()
  const hoursHeatmapMode = showDailyHours && !!getDailyHours
  const heatLegendGradient = useMemo(
    () => effortHeatmapGradientCss(heatmapMaxHours, resolvedTheme),
    [heatmapMaxHours, resolvedTheme],
  )

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 10, alignItems: 'center', flexWrap: 'wrap', marginBottom: 10 }}>
        <Space wrap>
          <Button size="small" onClick={() => onMonthChange(month.subtract(1, 'month'))}>
            上月
          </Button>
          <Button size="small" onClick={() => onMonthChange(dayjs())}>
            今天
          </Button>
          <Button size="small" onClick={() => onMonthChange(month.add(1, 'month'))}>
            下月
          </Button>
          <Text strong style={{ color: 'var(--zm-text-primary)' }}>
            {month.format('YYYY 年 M 月')}
          </Text>
          {loading ? <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>加载中…</Text> : null}
        </Space>
        <Segmented
          size="middle"
          value={mode}
          onChange={(v) => setMode(v as ViewMode)}
          options={[
            { label: '月视图', value: 'month' },
            { label: '列表', value: 'list' },
          ]}
        />
      </div>
      <Space size={12} wrap style={{ marginBottom: 10 }}>
        {getDailyHours ? (
          <Space size={6}>
            <Switch
              size="small"
              checked={showDailyHours}
              onChange={(checked) => {
                setShowDailyHours(checked)
                try {
                  localStorage.setItem(MY_WORKBENCH_CALENDAR_SHOW_HOURS_KEY, String(checked))
                } catch {
                  /* ignore quota / private mode */
                }
              }}
            />
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>报工工时</Text>
          </Space>
        ) : null}
        {hoursHeatmapMode ? (
          <Space size={8} align="center">
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 11 }}>少</Text>
            <span
              style={{
                display: 'inline-block',
                width: 120,
                height: 10,
                borderRadius: 4,
                background: heatLegendGradient,
                border: '1px solid var(--zm-border-subtle)',
              }}
            />
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 11 }}>多</Text>
            <Text style={{ color: 'var(--zm-text-muted)', fontSize: 11 }}>
              （满格 {heatmapMaxHours}h，超出{resolvedTheme === 'dark' ? '偏橙' : '偏红'}）
            </Text>
          </Space>
        ) : (
          <>
            <Space size={6}>
              <span
                style={{
                  width: 8,
                  height: 8,
                  borderRadius: 999,
                  background: CALENDAR_CATEGORY_COLORS.effort,
                  display: 'inline-block',
                }}
              />
              <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>禅道报工</Text>
            </Space>
            <Space size={6}>
              <span
                style={{
                  width: 8,
                  height: 8,
                  borderRadius: 999,
                  background: CALENDAR_CATEGORY_COLORS.taskPlan,
                  display: 'inline-block',
                }}
              />
              <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>任务计划</Text>
            </Space>
            <Space size={6}>
              <span
                style={{
                  width: 8,
                  height: 8,
                  borderRadius: 999,
                  background: CALENDAR_CATEGORY_COLORS.external,
                  display: 'inline-block',
                }}
              />
              <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>外部日历事件</Text>
            </Space>
          </>
        )}
      </Space>

      {mode === 'list' ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {listByDay.length === 0 ? (
            <div style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>本月无外部日历事件</div>
          ) : (
            listByDay.map((e) => {
              const span = daySpanInclusive(e._startDay, e._endDay)
              const hint = span > 1 ? `（跨 ${span} 天）` : ''
              return (
                <button
                  key={e._key}
                  onClick={() => setOpenEv(e)}
                  style={{
                    textAlign: 'left',
                    border: '1px solid var(--zm-border-subtle)',
                    background: 'var(--zm-bg-surface)',
                    borderRadius: 10,
                    padding: '10px 12px',
                    cursor: 'pointer',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <span
                      style={{
                        width: 10,
                        height: 10,
                        borderRadius: 3,
                        background: getCalendarEventDisplayColor(e),
                        flexShrink: 0,
                      }}
                    />
                    <Text style={{ color: 'var(--zm-text-primary)', fontWeight: 600 }}>
                      {e.title} <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>{hint}</Text>
                    </Text>
                  </div>
                  <div style={{ marginTop: 2, color: 'var(--zm-text-muted)', fontSize: 12 }}>
                    {e._startDay.format('MM-DD')} → {e._endDay.format('MM-DD')} · {e.source_name}
                  </div>
                </button>
              )
            })
          )}
        </div>
      ) : (
        <div style={{ border: '1px solid var(--zm-border-subtle)', borderRadius: 12, overflow: 'hidden', background: 'rgba(255,255,255,0.02)' }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', background: 'rgba(255,255,255,0.03)' }}>
            {weekdayLabels.map((w) => (
              <div key={w} style={{ padding: '8px 10px', color: 'var(--zm-text-muted)', fontSize: 12, borderBottom: '1px solid var(--zm-border-subtle)' }}>
                {w}
              </div>
            ))}
          </div>

          <div style={{ display: 'grid', gridTemplateRows: `repeat(${grid.weeks.length}, auto)` }}>
            {grid.weeks.map((week, wi) => {
              const weekStart = week[0]
              const { segs } = hoursHeatmapMode
                ? { segs: [] as WeekSeg[] }
                : packWeekSegments(weekStart, normalized, MAX_EVENT_LANES)
              return (
                <div key={weekStart.toString()} style={{ position: 'relative', borderBottom: wi === grid.weeks.length - 1 ? 'none' : '1px solid var(--zm-border-subtle)' }}>
                  <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)' }}>
                    {week.map((d) => {
                      const inMonth = d.isSame(month, 'month')
                      const isSelected = d.isSame(selectedDay, 'day')
                      const isToday = d.isSame(today, 'day')
                      const dots = !hoursHeatmapMode && getCellDots ? getCellDots(d) : null
                      const dailyHours = hoursHeatmapMode && getDailyHours ? Number(getDailyHours(d) ?? 0) : 0
                      const heat =
                        hoursHeatmapMode && Number.isFinite(dailyHours)
                          ? effortHeatmapStyle(dailyHours, heatmapMaxHours, resolvedTheme)
                          : null
                      const cellBg = heat
                        ? heat.background
                        : isSelected
                          ? 'rgba(22,119,255,0.14)'
                          : 'transparent'
                      const dateColor = heat
                        ? heat.textColor
                        : !inMonth
                          ? 'rgba(255,255,255,0.35)'
                          : 'var(--zm-text-primary)'
                      return (
                        <button
                          key={d.toString()}
                          onClick={() => onSelectDay(d)}
                          title={
                            hoursHeatmapMode
                              ? `${d.format('YYYY-MM-DD')} · 报工 ${Number.isFinite(dailyHours) ? dailyHours.toFixed(1) : '0'} 小时`
                              : undefined
                          }
                          style={{
                            position: 'relative',
                            display: 'flex',
                            flexDirection: 'column',
                            alignItems: hoursHeatmapMode ? 'center' : undefined,
                            justifyContent: hoursHeatmapMode ? 'center' : undefined,
                            minHeight: hoursHeatmapMode ? CELL_HEATMAP_MIN_HEIGHT : CELL_MIN_HEIGHT,
                            padding: hoursHeatmapMode ? '8px 6px' : 8,
                            border: 'none',
                            borderRight: d.day() === 6 ? 'none' : '1px solid var(--zm-border-subtle)',
                            background: cellBg,
                            boxShadow: isSelected
                              ? 'inset 0 0 0 2px rgba(22,119,255,0.85)'
                              : undefined,
                            cursor: 'pointer',
                            textAlign: hoursHeatmapMode ? 'center' : 'left',
                            opacity: hoursHeatmapMode && !inMonth ? 0.55 : 1,
                          }}
                        >
                          <span
                            style={{
                              position: 'absolute',
                              top: hoursHeatmapMode ? 6 : CELL_HEADER_TOP,
                              left: hoursHeatmapMode ? 6 : 8,
                              display: 'inline-flex',
                              alignItems: 'center',
                              justifyContent: 'center',
                              minWidth: 22,
                              height: 22,
                              padding: hoursHeatmapMode ? '0 5px' : 0,
                              borderRadius: 999,
                              fontSize: 12,
                              fontWeight: isToday ? 700 : 500,
                              color: dateColor,
                              background: heat
                                ? heat.dateBadgeBg
                                : isToday
                                  ? 'rgba(255,77,79,0.18)'
                                  : 'transparent',
                              border: isToday
                                ? '1px solid rgba(255,77,79,0.75)'
                                : heat
                                  ? `1px solid ${heat.dateBadgeBorder}`
                                  : '1px solid transparent',
                              boxShadow: heat ? '0 1px 3px rgba(0,0,0,0.25)' : isToday ? '0 0 0 1px rgba(255,77,79,0.2)' : undefined,
                              zIndex: 1,
                            }}
                          >
                            {d.date()}
                          </span>
                          {hoursHeatmapMode ? (
                            <span
                              style={{
                                fontSize: 15,
                                fontWeight: 700,
                                lineHeight: 1.2,
                                color: heat?.textColor ?? 'var(--zm-text-primary)',
                                textShadow: heat ? '0 1px 4px rgba(0,0,0,0.35)' : undefined,
                                marginTop: 4,
                              }}
                            >
                              {dailyHours.toFixed(1)}
                              <span
                                style={{
                                  fontSize: 11,
                                  fontWeight: 600,
                                  marginLeft: 1,
                                  color: heat?.hoursMuted ?? 'var(--zm-text-muted)',
                                }}
                              >
                                h
                              </span>
                            </span>
                          ) : null}
                          {dots && (dots.colors?.length ?? 0) > 0 ? (
                            <div
                              style={{
                                position: 'absolute',
                                top: CELL_HEADER_TOP + 8,
                                right: 8,
                                display: 'flex',
                                gap: 4,
                                flexWrap: 'wrap',
                                justifyContent: 'flex-end',
                                maxWidth: 'calc(100% - 42px)',
                              }}
                            >
                              {dots.colors.slice(0, 6).map((c, idx) => (
                                <span
                                  key={`${c}-${idx}`}
                                  style={{
                                    width: 6,
                                    height: 6,
                                    borderRadius: 999,
                                    background: c,
                                    display: 'inline-block',
                                    opacity: 0.95,
                                  }}
                                />
                              ))}
                              {typeof dots.n === 'number' && dots.n > dots.colors.length ? (
                                <span style={{ fontSize: 10, color: 'var(--zm-text-muted)', lineHeight: '6px' }}>
                                  +{dots.n - dots.colors.length}
                                </span>
                              ) : null}
                            </div>
                          ) : null}
                        </button>
                      )
                    })}
                  </div>

                  {/* event lanes overlay */}
                  {!hoursHeatmapMode ? (
                  <div
                    style={{
                      position: 'absolute',
                      left: 0,
                      right: 0,
                      top: CELL_OVERLAY_TOP,
                      padding: '0 6px',
                      pointerEvents: 'none',
                    }}
                  >
                    {segs.map((s) => {
                      const leftPct = (s.startIdx / 7) * 100
                      const widthPct = (s.span / 7) * 100
                      const barTop = s.lane * EVENT_ROW_STEP
                      const spanDays = daySpanInclusive(s.ev._startDay, s.ev._endDay)
                      const showSpan = spanDays > 1 && s.isEnd
                      const endHint = showSpan ? ` · ${spanDays}天` : ''
                      return (
                        <div
                          key={s.key}
                          onClick={() => setOpenEv(s.ev)}
                          style={{
                            pointerEvents: 'auto',
                            position: 'absolute',
                            left: `calc(${leftPct}% + 2px)`,
                            width: `calc(${widthPct}% - 4px)`,
                            top: barTop,
                            height: EVENT_BAR_HEIGHT,
                            background: getCalendarEventDisplayColor(s.ev),
                            color: 'white',
                            borderRadius: 6,
                            display: 'flex',
                            alignItems: 'center',
                            padding: '0 6px',
                            fontSize: 11,
                            lineHeight: `${EVENT_BAR_HEIGHT}px`,
                            boxShadow: '0 1px 0 rgba(0,0,0,0.25)',
                            opacity: 0.92,
                            cursor: 'pointer',
                            overflow: 'hidden',
                            whiteSpace: 'nowrap',
                            textOverflow: 'ellipsis',
                            userSelect: 'none',
                          }}
                          title={`${s.ev.title} (${s.ev._startDay.format('YYYY-MM-DD')} → ${s.ev._endDay.format('YYYY-MM-DD')})`}
                        >
                          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
                            {s.ev.title}
                            {endHint}
                          </span>
                        </div>
                      )
                    })}
                  </div>
                  ) : null}
                </div>
              )
            })}
          </div>
        </div>
      )}

      <Modal
        open={!!openEv}
        onCancel={() => setOpenEv(null)}
        footer={null}
        title="日程详情"
        width={680}
      >
        {openEv ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span
                style={{
                  width: 12,
                  height: 12,
                  borderRadius: 3,
                  background: getCalendarEventDisplayColor(openEv),
                }}
              />
              <Text strong style={{ color: 'var(--zm-text-primary)', fontSize: 16 }}>{openEv.title}</Text>
            </div>
            <div style={{ color: 'var(--zm-text-muted)', fontSize: 13 }}>
              {openEv._startDay.format('YYYY-MM-DD')} → {openEv._endDay.format('YYYY-MM-DD')}
              {' · '}
              {openEv.all_day ? '全天' : `${dayjs(openEv.start).format('HH:mm')}–${dayjs(openEv.end).format('HH:mm')}`}
              {' · '}
              {openEv.source_name}
            </div>
            <Space wrap>
              <Button size="small" onClick={() => { onSelectDay(openEv._startDay); setOpenEv(null) }}>
                跳到开始日
              </Button>
              {openEv.source_type === 'task' && openEv.source_id > 0 && getTaskDetailHref ? (
                <Link to={getTaskDetailHref(openEv.source_id)} onClick={() => setOpenEv(null)}>
                  <Button type="primary" size="small">查看任务详情</Button>
                </Link>
              ) : null}
            </Space>
          </div>
        ) : null}
      </Modal>
    </div>
  )
}

