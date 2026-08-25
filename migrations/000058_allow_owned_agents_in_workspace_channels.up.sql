CREATE OR REPLACE FUNCTION enforce_agent_channel_membership()
RETURNS trigger AS $$
DECLARE
    agent_kind VARCHAR(20);
    agent_home UUID;
    home_workspace UUID;
    channel_kind VARCHAR(20);
    target_workspace UUID;
BEGIN
    IF NEW.member_type <> 'agent' THEN
        RETURN NEW;
    END IF;

    SELECT a.kind, a.home_channel_id, home.workspace_id
      INTO agent_kind, agent_home, home_workspace
      FROM agents a
      LEFT JOIN channels home ON home.id = a.home_channel_id
     WHERE a.id = NEW.member_id;

    SELECT type, workspace_id
      INTO channel_kind, target_workspace
      FROM channels
     WHERE id = NEW.channel_id;

    IF channel_kind = 'dm' THEN
        RETURN NEW;
    END IF;

    IF agent_kind = 'lucy' THEN
        IF agent_home IS DISTINCT FROM NEW.channel_id OR channel_kind <> 'lucy' THEN
            RAISE EXCEPTION 'Lucy may only join her Lucy channel or a DM';
        END IF;
        RETURN NEW;
    END IF;

    IF channel_kind <> 'channel' THEN
        RAISE EXCEPTION 'ordinary agents may only join workspace channels or a DM';
    END IF;

    IF agent_home IS NULL OR home_workspace IS DISTINCT FROM target_workspace THEN
        RAISE EXCEPTION 'agent % belongs to another workspace', NEW.member_id;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
