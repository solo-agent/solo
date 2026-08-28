ALTER TABLE lark_bindings
  ADD COLUMN connection_mode VARCHAR(16) NOT NULL DEFAULT 'callback'
    CHECK (connection_mode IN ('callback', 'websocket')),
  ADD COLUMN connection_status VARCHAR(16) NOT NULL DEFAULT 'connected'
    CHECK (connection_status IN ('connecting', 'connected', 'error')),
  ADD COLUMN connection_error TEXT;
