import type React from 'react'
import type { ColumnsType } from 'antd/es/table'

export type ColumnMeta = {
  key: string
  title: string
  defaultWidth?: number
}

export type TableColumnPrefs = {
  order: string[]
  hidden: string[]
  widths?: Record<string, number>
}

export function getColumnKey(col: { key?: React.Key; dataIndex?: unknown }): string {
  const raw = col.key ?? col.dataIndex
  if (raw == null) return ''
  if (Array.isArray(raw)) return raw.map(String).join('.')
  return String(raw)
}

export function columnStorageKey(scopeUserId: number | string, tableId: string): string {
  return `zb.tableColumns.${scopeUserId}.${tableId}`
}

export function defaultWidthsFromMetas(metas: ColumnMeta[]): Record<string, number> {
  const out: Record<string, number> = {}
  for (const m of metas) {
    if (typeof m.defaultWidth === 'number' && m.defaultWidth > 0) out[m.key] = m.defaultWidth
  }
  return out
}

export function mergePrefsWithDefaults(
  prefs: TableColumnPrefs,
  defaultKeys: string[],
  defaultWidths: Record<string, number> = {},
): TableColumnPrefs {
  const hidden = prefs.hidden.filter((k) => defaultKeys.includes(k))
  const order = [
    ...prefs.order.filter((k) => defaultKeys.includes(k)),
    ...defaultKeys.filter((k) => !prefs.order.includes(k)),
  ]
  const widths: Record<string, number> = {}
  for (const key of defaultKeys) {
    const saved = prefs.widths?.[key]
    if (typeof saved === 'number' && saved > 0) widths[key] = saved
    else if (defaultWidths[key]) widths[key] = defaultWidths[key]
  }
  return { order, hidden, widths }
}

export function loadTableColumnPrefs(
  storageKey: string,
  defaultKeys: string[],
  defaultWidths: Record<string, number> = {},
): TableColumnPrefs {
  try {
    const raw = localStorage.getItem(storageKey)
    if (!raw) return mergePrefsWithDefaults({ order: [...defaultKeys], hidden: [] }, defaultKeys, defaultWidths)
    const parsed = JSON.parse(raw) as Partial<TableColumnPrefs>
    const order = Array.isArray(parsed.order) ? parsed.order.map(String) : [...defaultKeys]
    const hidden = Array.isArray(parsed.hidden) ? parsed.hidden.map(String) : []
    const widths: Record<string, number> = {}
    if (parsed.widths && typeof parsed.widths === 'object') {
      for (const [k, v] of Object.entries(parsed.widths)) {
        const n = Number(v)
        if (Number.isFinite(n) && n > 0) widths[k] = n
      }
    }
    return mergePrefsWithDefaults({ order, hidden, widths }, defaultKeys, defaultWidths)
  } catch {
    return mergePrefsWithDefaults({ order: [...defaultKeys], hidden: [] }, defaultKeys, defaultWidths)
  }
}

export function saveTableColumnPrefs(storageKey: string, prefs: TableColumnPrefs): void {
  try {
    localStorage.setItem(storageKey, JSON.stringify(prefs))
  } catch {
    /* ignore quota / private mode */
  }
}

export function applyTableColumnPrefs<T>(
  columns: ColumnsType<T>,
  prefs: TableColumnPrefs,
): ColumnsType<T> {
  const hidden = new Set(prefs.hidden)
  const colMap = new Map<string, ColumnsType<T>[number]>()

  for (const col of columns) {
    const key = getColumnKey(col)
    if (key) colMap.set(key, col)
  }

  const ordered: ColumnsType<T> = []
  for (const key of prefs.order) {
    if (hidden.has(key)) continue
    const col = colMap.get(key)
    if (!col) continue
    const w = prefs.widths?.[key] ?? (col as { width?: number }).width
    ordered.push(typeof w === 'number' && w > 0 ? { ...col, width: w } : col)
  }
  return ordered
}

export function withResizableColumns<T>(
  columns: ColumnsType<T>,
  prefs: TableColumnPrefs,
  onResize: (key: string, width: number) => void,
): ColumnsType<T> {
  return columns.map((col) => {
    const key = getColumnKey(col)
    const width = prefs.widths?.[key] ?? (col as { width?: number }).width
    if (!key || typeof width !== 'number' || width <= 0) return col
    return {
      ...col,
      width,
      onHeaderCell: () => ({
        width,
        minWidth: 48,
        onResize: (next: number) => onResize(key, next),
      }),
    }
  })
}
