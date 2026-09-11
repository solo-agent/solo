ALTER TABLE agent_selection_trials DROP CONSTRAINT agent_selection_trials_arm_check;
ALTER TABLE agent_selection_trials ADD CONSTRAINT agent_selection_trials_arm_check CHECK(arm IN ('baseline','candidate','receiver','reviewer'));
