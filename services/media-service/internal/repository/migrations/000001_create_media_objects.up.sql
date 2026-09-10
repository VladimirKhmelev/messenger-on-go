CREATE TABLE IF NOT EXISTS media_objects (
    id UUID PRIMARY KEY,
    chat_id UUID NOT NULL,
    uploader_id UUID NOT NULL,
    object_key TEXT NOT NULL UNIQUE,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    confirmed BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_media_objects_chat_id ON media_objects (chat_id);
