export type WorkflowStatus = 'pending' | 'scheduled' | 'running' | 'paused' | 'completed' | 'cancelled' | 'failed'
export type ActivityStatus = 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'waiting_human'

export interface Workflow {
  id: string
  name: string
  graph_name: string
  status: WorkflowStatus
  not_before?: string
  started_at?: string
  finished_at?: string
  current_node: string
  context: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface ActivityInstance {
  id: string
  workflow_id: string
  node_id: string
  activity_name: string
  status: ActivityStatus
  input: Record<string, unknown>
  output?: Record<string, unknown>
  error_msg?: string
  error_count: number
  max_retries: number
  started_at?: string
  finished_at?: string
  created_at: string
  updated_at: string
}

export interface GraphNode {
  id: string
  activity_name: string
  max_retries: number
  input_mapping?: string[]
  transitions: Transition[]
  is_human: boolean
}

export interface Transition {
  conditions: Condition[]
  next_node: string
  label?: string
}

export interface Condition {
  key: string
  operator: string
  value: unknown
}

export interface ActivityGraph {
  name: string
  start_node: string
  nodes: Record<string, GraphNode>
}
