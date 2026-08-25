CREATE OR REPLACE FUNCTION enforce_agent_channel_membership()
RETURNS trigger AS $$
DECLARE
    agent_kind VARCHAR(20);
    agent_home UUID;
    channel_kind VARCHAR(20);
BEGIN
    IF NEW.member_type <> 'agent' THEN
        RETURN NEW;
    END IF;

    SELECT kind, home_channel_id
      INTO agent_kind, agent_home
      FROM agents
     WHERE id = NEW.member_id;

    SELECT type
      INTO channel_kind
      FROM channels
     WHERE id = NEW.channel_id;

    IF channel_kind = 'dm' THEN
        RETURN NEW;
    END IF;

    IF agent_home IS DISTINCT FROM NEW.channel_id THEN
        RAISE EXCEPTION 'agent % belongs to channel %, not %',
            NEW.member_id, agent_home, NEW.channel_id;
    END IF;

    IF agent_kind = 'lucy' AND channel_kind <> 'lucy' THEN
        RAISE EXCEPTION 'Lucy may only join her Lucy channel or a DM';
    END IF;

    IF agent_kind = 'agent' AND channel_kind <> 'channel' THEN
        RAISE EXCEPTION 'ordinary agents may only join their home channel or a DM';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
