import React, { useEffect, useMemo, useState } from 'react'
import { TreeSelect, Typography, message } from 'antd'
import { getWorkbenchStructure } from '../api'

const { Text } = Typography

type TreeNode = {
  key: string
  type: string
  id: number
  title: string
  parent_id?: number
  children?: TreeNode[]
}

function flattenExecutions(nodes: TreeNode[], out: { id: number; title: string; parentId?: number }[] = []) {
  for (const n of nodes) {
    if (n.type === 'execution') out.push({ id: n.id, title: n.title, parentId: n.parent_id })
    if (n.children?.length) flattenExecutions(n.children, out)
  }
  return out
}

export const WorkbenchStructureSelect: React.FC<{
  value?: string
  onChange: (key: string | undefined, meta?: { type: string; id: number }) => void
}> = ({ value, onChange }) => {
  const [loading, setLoading] = useState(false)
  const [tree, setTree] = useState<TreeNode[]>([])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    getWorkbenchStructure()
      .then((r) => {
        if (cancelled) return
        setTree((r?.roots ?? []) as TreeNode[])
      })
      .catch((e: any) => {
        if (cancelled) return
        setTree([])
        message.error(e.response?.data?.error ?? '加载结构失败')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  const treeData = useMemo(() => {
    const mapNode = (n: TreeNode): any => ({
      value: n.key,
      title: n.title,
      selectable: n.type !== 'root',
      children: (n.children ?? []).map(mapNode),
    })
    return (tree ?? []).map(mapNode)
  }, [tree])

  const selectedMeta = useMemo(() => {
    const findByKey = (targetKey: string): { type: string; id: number } | undefined => {
      const walk = (nodes: TreeNode[]): { type: string; id: number } | undefined => {
        for (const n of nodes) {
          if (n.key === targetKey) return { type: n.type, id: n.id }
          const r = n.children ? walk(n.children) : undefined
          if (r) return r
        }
        return undefined
      }
      return walk(tree)
    }
    if (!value) return undefined
    return findByKey(value)
  }, [tree, value])

  const resolveMetaByKey = useMemo(() => {
    const walk = (nodes: TreeNode[], targetKey: string): { type: string; id: number } | undefined => {
      for (const n of nodes) {
        if (n.key === targetKey) return { type: n.type, id: n.id }
        const r = n.children ? walk(n.children, targetKey) : undefined
        if (r) return r
      }
      return undefined
    }
    return (targetKey?: string) => {
      if (!targetKey) return undefined
      return walk(tree, targetKey)
    }
  }, [tree])

  const hint = useMemo(() => {
    if (!selectedMeta) return '可选：项目集/项目/迭代，或产品线/产品'
    if (selectedMeta.type === 'execution') return '已按迭代筛选（联动各 Tab 的 execution_id）'
    if (selectedMeta.type === 'project') return '已按项目筛选（通过“项目→迭代”关系限制）'
    if (selectedMeta.type === 'program') return '已按项目集筛选（通过“项目集→项目→迭代”关系限制）'
    if (selectedMeta.type === 'product') return '已按产品筛选（通过需求 product_id 关联）'
    return '已选择'
  }, [selectedMeta])

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <TreeSelect
        allowClear
        showSearch
        treeLine
        loading={loading}
        value={value}
        onChange={(v) => {
          const key = (v as string) || undefined
          const meta = resolveMetaByKey(key)
          onChange(key, meta)
        }}
        placeholder="按项目/产品结构筛选"
        style={{ minWidth: 360 }}
        treeData={treeData}
        filterTreeNode={(input, node) => (String((node as any).title ?? '')).toLowerCase().includes(input.toLowerCase())}
      />
      <Text style={{ color: 'var(--zm-text-muted)', fontSize: 12 }}>{hint}</Text>
    </div>
  )
}
