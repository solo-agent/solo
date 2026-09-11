DROP TABLE agent_message_consumptions;
DROP TABLE agent_held_drafts;
DROP TABLE agent_work_marks;
DROP TRIGGER message_follow ON messages;
DROP FUNCTION follow_thread_on_reply();
DROP TABLE thread_subscriptions;
DROP TABLE agent_channel_attention;
ALTER TABLE agents DROP COLUMN attention_policy;
