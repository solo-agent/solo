// ============================================================================
// SOLO-21-F: WebSocket 消息类型定义
// SOLO-31-F: 扩展线程相关 WS 事件类型
// ============================================================================
// 后端所有 WS 通信使用 Envelope 格式: {"type": "...", "payload": {...}}
// 前端在 ws-client.ts 中解包服务端事件: payload 字段展开到顶层
// 前端在 ws-client.ts 中打包客户端命令: 非 type 字段包裹到 payload

import type { Attachment, Thread } from './types';

/** 消息来源（与后端 sender_type 对齐） */
export type WSMessageSource = 'user' | 'agent' | 'system';

/** 消息体（用于前端展示和状态管理） */
export interface WSMessage {
  id: string;
  channel_id: string;
  sender_type: WSMessageSource;
  sender_id: string;
  sender_name?: string;
  /** 发送者是否活跃（agent 被删除后为 false） */
  sender_active?: boolean;
  /** 前端渲染别名 */
  display_name?: string;
  user_id?: string;
  agent_id?: string;
  content: string;
  content_type?: string;
  thread_parent_id?: string;
  created_at: string;
  updated_at?: string;
  /** 发送状态（本地乐观更新 / 流式输出使用） */
  status?: 'sending' | 'sent' | 'failed' | 'streaming';
  /** SOLO-249-F: attachments on the message */
  attachments?: Attachment[];
}

/**
 * 服务端推送的 WebSocket 事件
 *
 * 后端发送 Envelope: {"type": "...", "payload": {...}}
 * ws-client.ts 的 handleMessage 会将 payload 展开到顶层,
 * 因此这里定义的类型都是扁平结构（不含 payload 嵌套字段）。
 */
export type WSServerEvent =
  | { type: 'connected'; user_id: string }
  | { type: 'error'; code: string; message: string }
  // ---- 消息事件 ----
  | { type: 'message.new'; id: string; channel_id: string; sender_type: string; sender_id: string; sender_name?: string; content: string; content_type: string; thread_id?: string; created_at: string; attachments?: Attachment[] }
  | { type: 'message.updated'; id: string; channel_id: string; content: string; sender_type?: string; sender_id?: string; updated_at: string; task_number?: number; task_title?: string; task_status?: string; task_claimer_name?: string; task_claimer_deleted?: boolean; reply_count?: number }
  | { type: 'message.deleted'; channel_id: string; message_id: string }
  // ---- 线程事件 ----
  // thread.message.new: 后端 ThreadMessageNewPayload 为 {message:{...}, thread:{...}} 嵌套结构
  | { type: 'thread.message.new'; message: { id: string; channel_id: string; thread_id: string; sender_type: string; sender_id: string; sender_name?: string; content: string; content_type: string; created_at: string }; thread: { thread_id: string; reply_count: number; last_reply_at: string } }
  // thread.reply: 后端 ThreadReplyNotifyPayload 包含 latest_reply 子对象
  | { type: 'thread.reply'; channel_id: string; thread_id: string; root_message_id?: string; reply_count: number; last_reply_at: string; latest_reply?: { id: string; sender_id: string; sender_name: string; content: string; created_at: string } }
  // ---- 输入状态 ----
  | { type: 'typing'; channel_id: string; user_id: string }
  // ---- Agent 状态事件 (SOLO-47-F) ----
  | { type: 'agent.thinking'; channel_id: string; agent_id: string; status: string; detail?: string; agent_name?: string; thought?: string }
  | { type: 'agent.typing'; channel_id: string; agent_id: string; status: string; detail?: string; agent_name?: string; thought?: string }
  | { type: 'agent.status'; channel_id: string; agent_id: string; status: string; detail?: string }
  | { type: 'agent.error'; channel_id: string; agent_id: string; status: string; detail?: string }
  // ---- Agent chunk events (agent view) ----
  | { type: 'agent.chunk'; channel_id: string; agent_id: string; agent_name: string; chunk_type: string; content: string; tool?: { name: string; input?: string; output?: string; call_id?: string }; timestamp: string }
  // ---- Agent done event (SOLO-island PR0) — terminal signal after a task
  // finishes, replaces the 3s inactivity heuristic. The frontend removes the
  // agent from the "active" list as soon as this arrives.
  | { type: 'agent.done'; channel_id: string; agent_id: string; agent_name?: string; task_id?: string; final_state: 'completed' | 'failed' | 'aborted' | 'timeout' | 'cancelled'; timestamp: string }
  // ---- Agent activity event (SOLO-island PR1) — single channel-scoped
  // event carrying the island-facing status and a one-line activity_text
  // summary. Powers the AgentIsland floating UI. Derived by the daemon
  // from agent.OutputChunk events.
  | { type: 'agent.activity'; channel_id: string; agent_id: string; agent_name?: string; status: 'idle' | 'thinking' | 'running' | 'streaming' | 'waiting_approval' | 'error'; activity_text: string; tool_name?: string; tool_input_summary?: string; source?: string; timestamp: string }
  // ---- 频道成员事件 ----
  | { type: 'member.added'; channel_id: string; member_type: string; member_id: string; member_name?: string }
  // ---- 流式消息事件 (SOLO-51-F, SOLO-52-F) ----
  | { type: 'message.agent_typing'; id: string; channel_id: string; thread_id?: string; sender_id: string; sender_name?: string; content: string; created_at: string }
  // ---- 任务事件 (SOLO-122-B) ----
  | { type: 'task.created'; id: string; task_number: number; channel_id: string; creator_id: string; title: string; description?: string; status: string; claimer_id?: string; priority?: string; due_date?: string; message_id?: string; parent_task_id?: string; subtask_count?: number; done_subtask_count?: number; created_at: string; updated_at: string }
  | { type: 'task.updated'; id: string; task_number: number; channel_id: string; title: string; description?: string; status: string; claimer_id?: string; claimer_name?: string; claimer_deleted?: boolean; priority?: string; due_date?: string; message_id?: string; parent_task_id?: string; subtask_count?: number; done_subtask_count?: number; updated_at: string }
  | { type: 'task.deleted'; id: string; channel_id: string; task_number: number }
  // ---- Task dependency events (Step 2) ----
  | { type: 'task.blocked'; blocked_task_id: string; blocker_task_id: string; channel_id: string }
  | { type: 'task.unblocked'; blocked_task_id: string; channel_id: string }
  // ---- Relationship events (Step 2 / Step 5) ----
  | { type: 'relationship_created'; id: string; from_agent_id: string; to_agent_id: string; rel_type: string; channel_id?: string }
  | { type: 'relationship_deleted'; id: string; from_agent_id: string; to_agent_id: string }
  | { type: 'relationship_updated'; id: string; from_agent_id: string; to_agent_id: string; rel_type: string; channel_id?: string }
  // ---- Agent deletion event (cascade) — soft-delete an agent, server
  // cascades its agent_relationships and broadcasts this so frontends can
  // prune local graph / list state without a refetch.
  | { type: 'agent_deleted'; agent_id: string; deleted_relationship_ids?: string[] }
  // ---- Channel memory events (Step 2) ----
  | { type: 'memory_updated'; channel_id: string }
  // ---- DM 事件 ----
  | { type: 'dm.message.new'; id: string; dm_id: string; sender_type: string; sender_id: string; sender_name?: string; content: string; content_type: string; created_at: string; attachments?: Attachment[]; thread_id?: string }
  | { type: 'dm.updated'; dm_id: string; last_message?: { content: string; sender_id: string; sender_name: string; created_at: string }; last_reply_at?: string; unread_count: number }
  // ---- Inbox events (v1.5) ----
  | { type: 'inbox.updated'; }
  // ---- Workspace conflict event (Step 3) ----
  | { type: 'workspace_conflict'; channel_id: string; task_id: string; agent_id: string; agent_name?: string; file_path: string; message: string; timestamp: string }
  // ---- Knowledge events (Step 4) ----
  | { type: 'knowledge_created'; entry_id: string; channel_id: string; title: string }
  | { type: 'knowledge_updated'; entry_id: string; channel_id: string; title: string }
  // ---- Step 6 events: reminder + watchdog + swarm ----
  | { type: 'reminder_fired'; reminder_id: string; agent_id: string; agent_name?: string; channel_id?: string; task_id?: string; message: string; fired_at: string }
  | { type: 'task_escalated'; task_id: string; task_number?: number; channel_id: string; escalated_to_agent_id: string; escalated_to_name?: string; level: 'yellow' | 'red'; timestamp: string }
  | { type: 'task_unclaimed_auto'; task_id: string; task_number?: number; channel_id: string; previous_claimer_id: string; reason: string; timestamp: string }
  | { type: 'swarm_decomposed'; parent_task_id: string; parent_task_number?: number; channel_id: string; subtask_count: number }
  | { type: 'swarm_all_done'; parent_task_id: string; parent_task_number?: number; channel_id: string; completed_count: number; timestamp: string };

/** 客户端发送的 WebSocket 命令 */
export type WSClientCommand =
  | { type: 'subscribe'; channel_id: string }
  | { type: 'unsubscribe'; channel_id: string }
  | { type: 'thread.subscribe'; thread_id: string }
  | { type: 'thread.unsubscribe'; thread_id: string }
  | { type: 'dm.subscribe'; dm_id: string }
  | { type: 'dm.unsubscribe'; dm_id: string }
  | { type: 'ping' };

/** WebSocket 连接状态 */
export type ConnectionState = 'disconnected' | 'connecting' | 'connected';
