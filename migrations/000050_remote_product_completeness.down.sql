DROP TABLE IF EXISTS channel_message_pins;
DROP TABLE IF EXISTS channel_member_mutes;
ALTER TABLE channels DROP COLUMN IF EXISTS posting_policy;
ALTER TABLE automations DROP COLUMN IF EXISTS completion_policy;
