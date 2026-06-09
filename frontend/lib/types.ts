// ============================================================================
// Shared types for Dashboard, Channels, Messages, and Agents
// ============================================================================

// ---- Attachment types (SOLO-247-F, SOLO-249-F) ----

export interface Attachment {
  id: string;
  filename: string;
  mime_type: string;
  size: number;
  url: string;
  thumbnail_url?: string;
}

export interface Channel {
  id: string;
  name: string;
  description: string;
  member_count: number;
  created_at: string;
  created_by: string;
}

export interface Message {
  id: string;
  channel_id: string;
  user_id: string;
  display_name: string;
  content: string;
  created_at: string;
  status: 'sending' | 'sent' | 'failed' | 'streaming';
  thread_parent_id?: string;
  /** 线程 ID (如果已有线程) */
  thread_id?: string;
  /** 线程回复数 (如果已有线程) */
  reply_count?: number;
  /** 消息来源类型（user / agent / system），用于区分渲染风格 (SOLO-48-F) */
  sender_type?: 'user' | 'agent' | 'system';
  /** 关联任务编号 (如果消息通过 asTask 转为任务) */
  task_number?: number;
  /** 关联任务标题 (从 tasks.title 获取) */
  task_title?: string;
  /** 关联任务状态 (todo / in_progress / in_review / done / closed) */
  task_status?: string;
  /** 关联任务认领人名称 */
  task_claimer_name?: string;
  /** 是否有未读的线程回复 (P25-08-F) */
  has_unread_thread?: boolean;
  /** 附件列表 (SOLO-249-F) */
  attachments?: Attachment[];
  /** 发送者是否活跃（agent 被删除后为 false） */
  sender_active?: boolean;
}

export interface CreateChannelInput {
  name: string;
  description?: string;
}

// ---- Agent types ----

export interface Agent {
  id: string;
  name: string;
  description: string;
  owner_id: string;
  model_provider: AgentModelProvider;
  model_name: string;
  system_prompt: string;
  temperature: number;
  max_tokens: number;
  is_active: boolean;
  auto_join: boolean;
  avatar_url: string | null;
  enabled_tools: string[];
  interaction_mode: 'active' | 'mention' | 'dnd';
  custom_env: Record<string, string>;
  custom_args: string[];
  created_at: string;
  updated_at: string;
}

/** v1.4: runtime type is dynamic, driven by backend registry (claude, codex, opencode, etc.) */
export type AgentModelProvider = string;

export interface CreateAgentInput {
  name: string;
  description?: string;
  model_provider: AgentModelProvider;
  model_name?: string;
  system_prompt?: string;
  temperature?: number;
  max_tokens?: number;
  avatar_url?: string;
  custom_env?: Record<string, string>;
  custom_args?: string[];
}

export interface UpdateAgentInput extends Partial<CreateAgentInput> {
  name?: string;
  enabled_tools?: string[];
  interaction_mode?: 'active' | 'mention' | 'dnd';
}

export type AgentInteractionMode = 'active' | 'mention' | 'dnd';

export interface AgentToolDef {
  id: string;
  name: string;
  description: string;
}

export const AVAILABLE_TOOLS: AgentToolDef[] = [
  { id: 'read_file', name: 'Read File', description: '读取服务器文件内容' },
  { id: 'write_file', name: 'Write File', description: '写入或编辑文件' },
  { id: 'list_files', name: 'List Files', description: '列出目录中的文件' },
  { id: 'search_files', name: 'Search Files', description: '搜索文件内容' },
];

// ---- Channel Member types ----

export interface ChannelMember {
  channel_id: string;
  member_type: 'user' | 'agent';
  member_id: string;
  role: 'owner' | 'admin' | 'member';
  display_name: string;
  status: 'online' | 'offline' | 'thinking' | 'typing';
}

// ---- Thread types ----

export interface Thread {
  id: string;
  channel_id: string;
  root_message_id: string;
  reply_count: number;
  last_reply_at: string;
}

// ---- Task types (SOLO-126-F, SOLO-127-F) ----

export type TaskStatus = 'todo' | 'in_progress' | 'in_review' | 'done' | 'closed';
export type TaskPriority = 'urgent' | 'high' | 'normal' | 'low';

export interface Task {
  id: string;
  channel_id: string;
  channel_name?: string;
  title: string;
  description: string;
  status: TaskStatus;
  priority: TaskPriority;
  /** 自动递增任务编号 (#1, #2, ...) */
  task_number?: number;
  /** @deprecated 使用 claimer_id 替代 (Phase 1 认领制改造) */
  assignee_id?: string;
  /** @deprecated 使用 claimer_name 替代 */
  assignee_name?: string;
  /** @deprecated 使用 claimer_type 替代 */
  assignee_type?: 'user' | 'agent';
  /** 认领人 ID (认领制) */
  claimer_id?: string;
  /** 认领人名称 */
  claimer_name?: string;
  /** 认领人类型 (user / agent) */
  claimer_type?: 'user' | 'agent';
  creator_id: string;
  creator_name?: string;
  /** 关联的消息 ID (任务即消息模型) */
  message_id?: string;
  /** 线程回复数 (后端可返回) */
  reply_count?: number;
  /** 父任务 ID (子任务指向父任务) */
  parent_task_id?: string | null;
  /** 子任务总数 (父任务，后端聚合) */
  subtask_count?: number;
  /** 已完成的子任务数 (父任务，后端聚合) */
  done_subtask_count?: number;
  due_date?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateTaskInput {
  channel_id: string;
  title: string;
  description?: string;
  priority?: TaskPriority;
  assignee_id?: string;
  assignee_type?: 'user' | 'agent';
  due_date?: string;
}

export interface UpdateTaskInput {
  title?: string;
  description?: string;
  status?: TaskStatus;
  priority?: TaskPriority;
  assignee_id?: string;
  assignee_type?: 'user' | 'agent';
  /** 认领人 ID (认领制) */
  claimer_id?: string;
  /** 认领人名称 */
  claimer_name?: string;
  due_date?: string;
}

// ---- DM types ----

/** 与另一个用户或 Agent 的私信频道 */
export interface DMChannel {
  id: string;
  type: 'dm';
  other_user?: {
    id: string;
    display_name: string;
    avatar_url?: string;
  };
  other_agent?: {
    id: string;
    name: string;
    avatar_url?: string;
    is_active?: boolean;
  };
  last_message?: {
    content: string;
    sender_id: string;
    sender_name: string;
    created_at: string;
  };
  last_reply_at?: string;
  unread_count: number;
  created_at: string;
}

export interface CreateDMInput {
  /** 发起私信的目标用户 ID（user 类型） */
  user_id?: string;
  /** 发起私信的目标 Agent ID（agent 类型） */
  agent_id?: string;
}

// ---- Computer types (SOLO-245-F, SOLO-246-F) ----

export interface Computer {
  id: string;
  name: string;
  owner_id: string;
  daemon_id?: string;
  daemon_url?: string;
  status: 'online' | 'offline';
  last_heartbeat?: string;
  agent_ids?: string[];
  agent_names?: string[];
  os?: string;
  hostname?: string;
  ip?: string;
  created_at: string;
  updated_at: string;
}

export interface UpdateComputerInput {
  name?: string;
}

// ---- Search types (SOLO-237-F) ----

/** Matches backend handler/search.go SearchResult JSON shape */
export interface SearchResult {
  id: string;
  channel_id: string;
  channel_name: string;
  sender_type: string;
  sender_id: string;
  sender_name: string;
  content: string;
  /** HTML snippet with highlighted matches via ts_headline */
  highlight: string;
  content_type: string;
  created_at: string;
}

/** Matches backend handler/search.go SearchResponse JSON shape */
export interface SearchResponse {
  results: SearchResult[];
  next_cursor: string | null;
  has_more: boolean;
  total_approx: number;
}

// ---- Agent Backend / CLI Detection types (v1.4) ----

/** Registered agent backend metadata from GET /api/v1/agent-backends */
export interface AgentBackendMeta {
  type: string;
  display_name: string;
  requires_binary: string;
  protocols: string[];
}

/** CLI detection item from GET /api/v1/agent-backends/detect */
export interface AgentBackendDetectItem {
  type: string;
  display_name: string;
  binary: string;
  available: boolean;
  version?: string;
  error?: string;
}

/** @deprecated v1.4 — use AgentBackendMeta instead */
export interface AgentBackendInfo {
  provider: AgentModelProvider;
  label: string;
  description: string;
  binary: string;
  install_hint?: string;
}

/** @deprecated v1.4 — use AgentBackendDetectItem instead */
export interface AgentBackendDetectResult {
  provider: AgentModelProvider;
  available: boolean;
  binary: string;
  version?: string;
  install_hint?: string;
}

// ---- Inbox types (v1.5) ----

export interface InboxItem {
  id: string;
  type: 'thread_reply' | 'dm' | 'mention';
  channel_id?: string | null;
  channel_name?: string | null;
  thread_id?: string | null;
  dm_id?: string | null;
  message_id: string;
  sender_name: string;
  sender_avatar?: string | null;
  content_preview: string;
  is_mention: boolean;
  is_unread: boolean;
  created_at: string;
  parent_sender_name?: string | null;
  parent_sender_type?: string | null;
  parent_sender_id?: string | null;
  parent_content?: string | null;
}

export interface UnreadCount {
  total: number;
  mentions: number;
  thread_replies: number;
  dm: number;
}

// ---- Workspace types (v1.5) ----

export interface WorkspaceFileNode {
  name: string;
  path: string;
  type: 'file' | 'directory';
  size?: number;
  children?: WorkspaceFileNode[];
}

// ---- Computer Agent types (v1.5) ----

export interface ComputerAgent {
  id: string;
  name: string;
  status: 'online' | 'thinking' | 'running' | 'offline';
  active_tasks: number;
}

// ---- Onboarding types (v1.6) ----

export interface CreateLucyRequest {
  runtime_type: string;
  computer_id?: string;
  channel_id: string;
}

export interface CreateLucyResponse {
  agent_id: string;
  agent_name: string;
  channel_id: string;
}

export type WizardStep = 'computer' | 'runtime' | 'create' | 'done';
