import React, { useRef } from 'react'

export type ResizeHeaderCellProps = React.ThHTMLAttributes<HTMLTableCellElement> & {
  width?: number
  minWidth?: number
  onResize?: (width: number) => void
}

/** 表头右侧拖拽调整列宽（配合 Table components.header.cell） */
export const ResizableTableHeaderCell: React.FC<ResizeHeaderCellProps> = ({
  width,
  minWidth = 48,
  onResize,
  children,
  style,
  ...rest
}) => {
  const startX = useRef(0)
  const startW = useRef(0)

  if (!width || !onResize) {
    return (
      <th {...rest} style={style}>
        {children}
      </th>
    )
  }

  const onMouseDown = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    startX.current = e.clientX
    startW.current = width

    const onMove = (ev: MouseEvent) => {
      onResize(Math.max(minWidth, Math.round(startW.current + ev.clientX - startX.current)))
    }
    const onUp = () => {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }

  return (
    <th
      {...rest}
      style={{
        ...style,
        width,
        minWidth: width,
        maxWidth: width,
        position: 'relative',
      }}
    >
      {children}
      <span
        role="separator"
        aria-orientation="vertical"
        aria-label="调整列宽"
        onMouseDown={onMouseDown}
        style={{
          position: 'absolute',
          right: 0,
          top: 0,
          bottom: 0,
          width: 8,
          cursor: 'col-resize',
          zIndex: 2,
          touchAction: 'none',
        }}
      />
    </th>
  )
}

export const resizableTableComponents = {
  header: {
    cell: ResizableTableHeaderCell,
  },
}
