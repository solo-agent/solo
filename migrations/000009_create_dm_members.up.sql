-- DM 参与者关系表
-- DM（私信）复用 channels 表（type='dm'），通过 dm_members 表记录参与关系。
-- DM 固定为 2 人（用户 <-> 用户 或 用户 <-> Agent）。

CREATE TABLE dm_members (
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    member_type     VARCHAR(20) NOT NULL,  -- 'user', 'agent'
    member_id       UUID NOT NULL,          -- references users(id) or agents(id)
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (channel_id, member_type, member_id)
);

-- 按 channel 查询 DM 参与者
CREATE INDEX idx_dm_members_channel ON dm_members(channel_id);

-- 查询用户/Agent 参与的所有 DM
CREATE INDEX idx_dm_members_lookup ON dm_members(member_type, member_id, channel_id);
