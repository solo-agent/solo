// ============================================================================
// SOLO-21-F: WebSocket 消息类型定义
// SOLO-31-F: 扩展线程相关 WS 事件类型
// ============================================================================
// 后端所有 WS 通信使用 Envelope 格式: {"type": "...", "payload": {...}}
// 前端在 ws-client.ts 中解包服务端事件: payload 字段展开到顶层
// 前端在 ws-client.ts 中打包客户端命令: 非 type 字段包裹到 payload

import type { Attachment } from './types';

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
  thinking_node_id?: string;
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
  | { type: 'message.new'; id: string; channel_id: string; sender_type: string; sender_id: string; sender_name?: string; content: string; content_type: string; thread_id?: string; thinking_node_id?: string; created_at: string; attachments?: Attachment[] }
  | { type: 'message.updated'; id: string; channel_id: string; content: string; sender_type?: string; sender_id?: string; updated_at: string; task_number?: number; task_title?: string; task_status?: string; task_claimer_name?: string; task_claimer_deleted?: boolean; reply_count?: number }
  | { type: 'message.deleted'; channel_id: string; message_id: string }
  | { type: 'thinking.updated'; channel_id: string; space_id?: string; node_id?: string }
  // ---- 线程事件 ----
  // thread.message.new: 后端 ThreadMessageNewPayload 为 {message:{...}, thread:{...}} 嵌套结构
  | { type: 'thread.message.new'; message: { id: string; channel_id: string; thread_id: string; sender_type: string; sender_id: string; sender_name?: string; content: string; content_type: string; created_at: string; attachments?: Attachment[] }; thread: { thread_id: string; reply_count: number; last_reply_at: string } }
  // thread.reply: 后端 ThreadReplyNotifyPayload 包含 latest_reply 子对象
  | { type: 'thread.reply'; channel_id: string; thread_id: string; root_message_id?: string; reply_count: number; last_reply_at: string; latest_reply?: { id: string; sender_id: string; sender_name: string; content: string; created_at: string } }
  // ---- 输入状态 ----
  | { type: 'typing'; channel_id: string; user_id: string }
  // ---- Agent 状态事件 (SOLO-47-F) ----
  | { type: 'agent.thinking'; channel_id: string; agent_id: string; status: string; detail?: string; agent_name?: string; thought?: string }
  | { type: 'agent.typing'; channel_id: string; agent_id: string; status: string; detail?: string; agent_name?: string; thought?: string }
  | { type: 'agent.status'; channel_id: string; agent_id: string; status: string; detail?: string }
  | { type: 'agent.error'; channel_id: string; thread_id?: string; agent_id: string; agent_name?: string; error?: string; detail?: string }
  // ---- Agent chunk events (agent view) ----
  | { type: 'agent.chunk'; channel_id: string; agent_id: string; agent_name: string; chunk_type: string; content: string; tool?: { name: string; input?: string; output?: string; call_id?: string }; timestamp: string }
  | { type: 'agent.run.started'; run_id: string; session_id?: string; agent_id: string; agent_name?: string; task_id?: string; channel_id?: string; thread_id?: string; thinking_node_id?: string; status: 'queued' | 'thinking' | 'running' | 'streaming' | 'waiting_input' | 'waiting_approval' | 'completed' | 'failed' | 'cancelled' | 'timeout'; activity_text?: string; tool_name?: string; tool_input_summary?: string; transcript_path?: string; source?: string; backend_started_at?: string; timestamp?: string }
  | { type: 'agent.run.updated'; run_id: string; session_id?: string; agent_id: string; agent_name?: string; task_id?: string; channel_id?: string; thread_id?: string; thinking_node_id?: string; status: 'queued' | 'thinking' | 'running' | 'streaming' | 'waiting_input' | 'waiting_approval' | 'completed' | 'failed' | 'cancelled' | 'timeout'; activity_text?: string; tool_name?: string; tool_input_summary?: string; transcript_path?: string; source?: string; backend_started_at?: string; timestamp?: string }
  | { type: 'agent.run.finished'; run_id: string; session_id?: string; agent_id: string; agent_name?: string; task_id?: string; channel_id?: string; thread_id?: string; thinking_node_id?: string; status: 'completed' | 'failed' | 'cancelled' | 'timeout'; activity_text?: string; tool_name?: string; tool_input_summary?: string; transcript_path?: string; source?: string; backend_started_at?: string; timestamp?: string }
  | { type: 'agent.run.event'; id?: string; run_id: string; session_id?: string; agent_id: string; agent_name?: string; channel_id?: string; thread_id?: string; thinking_node_id?: string; seq: number; event_type: string; message?: string; tool_name?: string; payload?: Record<string, unknown>; timestamp: string }
  // ---- 频道成员事件 ----
  | { type: 'member.added'; channel_id: string; member_type: string; member_id: string; member_name?: string }
  | { type: 'member.removed'; channel_id: string; member_type?: string; member_id: string; member_name?: string }
  // ---- 流式消息事件 (SOLO-51-F, SOLO-52-F) ----
  | { type: 'message.agent_typing'; id: string; channel_id: string; thread_id?: string; thinking_node_id?: string; sender_id: string; sender_name?: string; content: string; created_at: string }
  // ---- 任务事件 (SOLO-122-B) ----
  | { type: 'task.created'; id: string; task_number: number; channel_id: string; creator_id: string; title: string; description?: string; status: string; claimer_id?: string; priority?: string; due_date?: string; message_id?: string; parent_task_id?: string; subtask_count?: number; done_subtask_count?: number; artifact_status?: 'none' | 'pending' | 'available'; created_at: string; updated_at: string }
  | { type: 'task.updated'; id: string; task_number: number; channel_id: string; title: string; description?: string; status: string; claimer_id?: string; claimer_name?: string; claimer_deleted?: boolean; priority?: string; due_date?: string; message_id?: string; parent_task_id?: string; subtask_count?: number; done_subtask_count?: number; artifact_status?: 'none' | 'pending' | 'available'; updated_at: string }
  | { type: 'task.deleted'; id: string; channel_id: string; task_number: number }
  // ---- Relationship events ----
  | { type: 'relationship_created'; id: string; from_agent_id: string; to_agent_id: string; rel_type: string; channel_id?: string; channel_name?: string }
  | { type: 'relationship_updated'; id: string; from_agent_id: string; to_agent_id: string; rel_type: string; channel_id?: string; channel_name?: string }
  | { type: 'relationship_deleted'; id: string; from_agent_id: string; to_agent_id: string }
  | { type: 'agent_deleted'; agent_id: string }
  // ---- DM 事件 ----
  | { type: 'dm.message.new'; id: string; dm_id: string; sender_type: string; sender_id: string; sender_name?: string; content: string; content_type: string; created_at: string; attachments?: Attachment[]; thread_id?: string }
  | { type: 'dm.updated'; dm_id: string; last_message?: { content: string; sender_id: string; sender_name: string; created_at: string }; last_reply_at?: string; unread_count: number }
  // ---- Inbox events (v1.5) ----
  | { type: 'inbox.updated'; }
  // ---- Lucy automatic team formation ----
  | { type: 'team.formed'; formation_id: string; source_channel_id: string; source_message_id: string; channel_id: string; channel_name: string; member_count: number; task_count: number; relationship_template?: string; relationship_overrides?: number; relationship_docs_ready: boolean; warnings?: string[]; dashboard_url: string; created_at: string };

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
