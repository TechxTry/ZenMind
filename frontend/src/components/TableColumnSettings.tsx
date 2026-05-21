import React from 'react'
import { ArrowDownOutlined, ArrowUpOutlined, ColumnHeightOutlined } from '@ant-design/icons'
import { Button, Checkbox, Divider, Popover, Space, Typography } from 'antd'
import type { ColumnMeta, TableColumnPrefs } from '../utils/tableColumnPrefs'

const { Text } = Typography

type Props = {
  metas: ColumnMeta[]
  prefs: TableColumnPrefs
  onChange: (prefs: TableColumnPrefs) => void
  onReset: () => void
  size?: 'small' | 'middle'
}

function moveKey(order: string[], key: string, delta: -1 | 1): string[] {
  const idx = order.indexOf(key)
  if (idx < 0) return order
  const nextIdx = idx + delta
  if (nextIdx < 0 || nextIdx >= order.length) return order
  const next = [...order]
  ;[next[idx], next[nextIdx]] = [next[nextIdx], next[idx]]
  return next
}

const TableColumnSettings: React.FC<Props> = ({ metas, prefs, onChange, onReset, size = 'small' }) => {
  const metaByKey = React.useMemo(() => new Map(metas.map((m) => [m.key, m])), [metas])
  const orderedConfigurable = prefs.order.filter((k) => metaByKey.has(k))

  const content = (
    <div style={{ width: 240 }}>
      <Text style={{ fontSize: 12, color: 'var(--zb-text-muted)' }}>
        勾选显示列、箭头调顺序；表头右侧拖拽可调列宽（仅本账号）
      </Text>
      <Divider style={{ margin: '8px 0' }} />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        {orderedConfigurable.map((key, idx) => {
          const meta = metaByKey.get(key)
          if (!meta) return null
          const visible = !prefs.hidden.includes(key)
          return (
            <div key={key} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <Checkbox
                checked={visible}
                onChange={(e) => {
                  const hidden = new Set(prefs.hidden)
                  if (e.target.checked) hidden.delete(key)
                  else hidden.add(key)
                  onChange({ ...prefs, hidden: [...hidden] })
                }}
              >
                <span style={{ fontSize: 13 }}>{meta.title}</span>
              </Checkbox>
              <Space size={0} style={{ marginLeft: 'auto' }}>
                <Button
                  type="text"
                  size="small"
                  icon={<ArrowUpOutlined />}
                  disabled={idx === 0}
                  onClick={() => onChange({ ...prefs, order: moveKey(prefs.order, key, -1) })}
                />
                <Button
                  type="text"
                  size="small"
                  icon={<ArrowDownOutlined />}
                  disabled={idx === orderedConfigurable.length - 1}
                  onClick={() => onChange({ ...prefs, order: moveKey(prefs.order, key, 1) })}
                />
              </Space>
            </div>
          )
        })}
      </div>
      <Divider style={{ margin: '8px 0' }} />
      <Button type="link" size="small" style={{ padding: 0 }} onClick={onReset}>
        恢复默认
      </Button>
    </div>
  )

  return (
    <Popover title="列设置" trigger="click" placement="bottomRight" content={content}>
      <Button size={size} icon={<ColumnHeightOutlined />}>
        列
      </Button>
    </Popover>
  )
}

export default TableColumnSettings
