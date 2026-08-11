CREATE TABLE IF NOT EXISTS message_events (
    id UUID PRIMARY KEY,
    message_id UUID NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    chat_id UUID NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
    actor_id UUID NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('edited', 'deleted_for_all')),
    new_body TEXT,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_message_events_message_id_created_at
    ON message_events (message_id, created_at);

CREATE TABLE IF NOT EXISTS message_hidden_for_user (
    message_id UUID NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    hidden_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (message_id, user_id)
);
