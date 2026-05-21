import React from 'react'
import { SettingOutlined } from '@ant-design/icons'
import { Button, Divider, Popover, Radio, Typography } from 'antd'
import type { WeekStartsOn } from '../utils/myWorkbenchCalendarPrefs'

const { Text } = Typography

type Props = {
  weekStartsOn: WeekStartsOn
  onWeekStartsOnChange: (value: WeekStartsOn) => void
}

const MyWorkbenchSettings: React.FC<Props> = ({ weekStartsOn, onWeekStartsOnChange }) => {
  const content = (
    <div style={{ width: 260 }}>
      <Text style={{ fontSize: 12, color: 'var(--zm-text-muted)' }}>仅保存在本浏览器，换设备需重新设置。</Text>
      <Divider style={{ margin: '10px 0' }} />
      <Text strong style={{ display: 'block', marginBottom: 8, color: 'var(--zm-text-primary)' }}>
        日历
      </Text>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12 }}>
        <Text style={{ color: 'var(--zm-text-primary)', fontSize: 13 }}>每周开始于</Text>
        <Radio.Group
          size="small"
          value={weekStartsOn}
          onChange={(e) => onWeekStartsOnChange(e.target.value as WeekStartsOn)}
          optionType="button"
          buttonStyle="solid"
          options={[
            { label: '周日', value: 0 },
            { label: '周一', value: 1 },
          ]}
        />
      </div>
    </div>
  )

  return (
    <Popover title="工作台设置" trigger="click" placement="bottomLeft" content={content}>
      <Button
        type="text"
        size="small"
        icon={<SettingOutlined />}
        aria-label="工作台设置"
        style={{ color: 'var(--zm-text-muted)', marginLeft: 4, verticalAlign: 'middle' }}
      />
    </Popover>
  )
}

export default MyWorkbenchSettings
