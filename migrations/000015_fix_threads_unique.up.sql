-- Add unique constraint on threads.root_message_id to prevent duplicate thread entries
-- First remove any remaining duplicates (keep the one with most recent activity)
DELETE FROM threads
WHERE id IN (
  SELECT id FROM (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY root_message_id ORDER BY last_reply_at DESC NULLS LAST) as rn
    FROM threads
  ) sub
  WHERE sub.rn > 1
);

ALTER TABLE threads ADD CONSTRAINT threads_root_message_id_unique UNIQUE (root_message_id);
