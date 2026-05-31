CREATE TABLE thread_reads (
    user_id UUID NOT NULL,
    thread_id UUID NOT NULL,
    last_read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, thread_id),
    CONSTRAINT fk_thread FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE
);
