import { useCallback, useEffect, useMemo, useState } from 'react'
import type { ColumnsType } from 'antd/es/table'
import {
  applyTableColumnPrefs,
  columnStorageKey,
  defaultWidthsFromMetas,
  getColumnKey,
  loadTableColumnPrefs,
  mergePrefsWithDefaults,
  saveTableColumnPrefs,
  withResizableColumns,
  type ColumnMeta,
  type TableColumnPrefs,
} from '../utils/tableColumnPrefs'

export function useTableColumnPrefs<T>(
  userId: number | undefined,
  tableId: string,
  allColumns: ColumnsType<T>,
  metas: ColumnMeta[],
) {
  const defaultKeys = useMemo(() => metas.map((m) => m.key), [metas])
  const defaultWidths = useMemo(() => {
    const fromMetas = defaultWidthsFromMetas(metas)
    for (const col of allColumns) {
      const key = getColumnKey(col)
      const w = (col as { width?: number }).width
      if (key && typeof w === 'number' && w > 0 && !fromMetas[key]) fromMetas[key] = w
    }
    return fromMetas
  }, [metas, allColumns])

  const storageKey = userId != null ? columnStorageKey(userId, tableId) : null

  const [prefs, setPrefsState] = useState<TableColumnPrefs>(() =>
    mergePrefsWithDefaults({ order: defaultKeys, hidden: [] }, defaultKeys, defaultWidths),
  )

  useEffect(() => {
    if (!storageKey) {
      setPrefsState(mergePrefsWithDefaults({ order: defaultKeys, hidden: [] }, defaultKeys, defaultWidths))
      return
    }
    setPrefsState(loadTableColumnPrefs(storageKey, defaultKeys, defaultWidths))
  }, [storageKey, defaultKeys.join('|'), JSON.stringify(defaultWidths)])

  const setPrefs = useCallback(
    (next: TableColumnPrefs) => {
      const merged = mergePrefsWithDefaults(next, defaultKeys, defaultWidths)
      setPrefsState(merged)
      if (storageKey) saveTableColumnPrefs(storageKey, merged)
    },
    [storageKey, defaultKeys, defaultWidths],
  )

  const setColumnWidth = useCallback(
    (key: string, width: number) => {
      setPrefsState((prev) => {
        const merged = mergePrefsWithDefaults(
          { ...prev, widths: { ...prev.widths, [key]: width } },
          defaultKeys,
          defaultWidths,
        )
        if (storageKey) saveTableColumnPrefs(storageKey, merged)
        return merged
      })
    },
    [storageKey, defaultKeys, defaultWidths],
  )

  const resetPrefs = useCallback(() => {
    const fresh = mergePrefsWithDefaults({ order: [...defaultKeys], hidden: [] }, defaultKeys, defaultWidths)
    setPrefsState(fresh)
    if (storageKey) saveTableColumnPrefs(storageKey, fresh)
  }, [storageKey, defaultKeys, defaultWidths])

  const columns = useMemo(() => {
    const ordered = applyTableColumnPrefs(allColumns, prefs)
    return withResizableColumns(ordered, prefs, setColumnWidth)
  }, [allColumns, prefs, setColumnWidth])

  return { columns, prefs, setPrefs, setColumnWidth, resetPrefs, metas }
}
