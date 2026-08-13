-- Workspace membership is the User visibility boundary. Materialize every
-- active Workspace User into each active ordinary Channel so existing Channel
-- authorization, WebSocket, message, and member-list paths stay consistent.
-- Lucy Channels and DMs remain participant-scoped; Agent rows remain explicit.
INSERT INTO channel_members (channel_id, member_type, member_id, role)
SELECT c.id,
       'user',
       wm.user_id,
       CASE WHEN c.created_by = wm.user_id THEN 'owner' ELSE 'member' END
  FROM channels c
  JOIN workspace_members wm ON wm.workspace_id = c.workspace_id
  JOIN users u ON u.id = wm.user_id AND u.is_active = true
 WHERE c.type = 'channel'
   AND c.is_archived = false
ON CONFLICT (channel_id, member_type, member_id) DO NOTHING;

-- Historical seeded Channels can reference a now-inactive creator. Ensure
-- every active ordinary Channel still has an active User owner, preferring the
-- Workspace owner and then the earliest active Workspace member.
WITH ownerless AS (
    SELECT c.id
      FROM channels c
     WHERE c.type = 'channel'
       AND c.is_archived = false
       AND NOT EXISTS (
           SELECT 1
             FROM channel_members cm
             JOIN users u ON u.id = cm.member_id AND u.is_active = true
            WHERE cm.channel_id = c.id
              AND cm.member_type = 'user'
              AND cm.role = 'owner'
       )
), replacement AS (
    SELECT DISTINCT ON (c.id) c.id AS channel_id, wm.user_id
      FROM channels c
      JOIN ownerless missing ON missing.id = c.id
      JOIN workspace_members wm ON wm.workspace_id = c.workspace_id
      JOIN users u ON u.id = wm.user_id AND u.is_active = true
     ORDER BY c.id,
              CASE wm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,
              wm.joined_at,
              wm.user_id
)
UPDATE channel_members cm
   SET role = 'owner'
  FROM replacement chosen
 WHERE cm.channel_id = chosen.channel_id
   AND cm.member_type = 'user'
   AND cm.member_id = chosen.user_id;
