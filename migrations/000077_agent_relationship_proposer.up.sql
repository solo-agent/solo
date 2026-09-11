ALTER TABLE agent_relationship_proposals ADD COLUMN proposed_by_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL;
COMMENT ON COLUMN agent_relationship_proposals.proposed_by_agent_id IS 'Actual Agent initiator; proposed_by remains its owner and does not imply owner approval';
