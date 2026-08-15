CREATE TABLE IF NOT EXISTS user_avatars (
    user_id UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    data BYTEA NOT NULL,
    content_type TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
