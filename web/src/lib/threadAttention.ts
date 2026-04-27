import type { ExecutionNode } from '../api/client'

export function getHumanAttentionNodes(nodes: ExecutionNode[]): ExecutionNode[] {
  return nodes.filter((node) => Boolean(node.awaiting_human_input))
}

export function getHumanAttentionRootIds(nodes: ExecutionNode[]): Set<string> {
  const roots = new Set<string>()
  for (const node of getHumanAttentionNodes(nodes)) {
    const rootId = (node.root_message_id || '').trim()
    if (rootId) roots.add(rootId)
  }
  return roots
}

export function getHumanAttentionReason(nodes: ExecutionNode[]): string | null {
  for (const node of getHumanAttentionNodes(nodes)) {
    const reason = (node.awaiting_human_reason || '').trim()
    if (reason) return reason
  }
  return null
}
