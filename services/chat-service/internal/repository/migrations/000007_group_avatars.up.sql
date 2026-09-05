CREATE TABLE IF NOT EXISTS chat_avatars (
    chat_id UUID PRIMARY KEY REFERENCES chats (id) ON DELETE CASCADE,
    data BYTEA NOT NULL,
    content_type TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
