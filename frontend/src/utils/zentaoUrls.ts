/** 禅道 Web 任务详情页（与 probe 中 task-recordEstimate 等同级的 base 前缀）。 */
export function buildZentaoTaskViewUrl(
  baseUrl: string,
  taskId: number,
  executionId?: number | null,
): string | null {
  const base = baseUrl.trim().replace(/\/+$/, '')
  if (!base || !Number.isFinite(taskId) || taskId <= 0) return null
  const path =
    executionId != null && Number(executionId) > 0
      ? `execution-task-${taskId}.html`
      : `task-view-${taskId}.html`
  return `${base}/${path}`
}
