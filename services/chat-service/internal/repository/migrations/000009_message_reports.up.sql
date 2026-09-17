CREATE TABLE IF NOT EXISTS message_reports (
    id UUID PRIMARY KEY,
    message_id UUID NOT NULL,
    chat_id UUID NOT NULL,
    reporter_id UUID NOT NULL,
    category TEXT NOT NULL,
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (message_id, reporter_id)
);

CREATE INDEX IF NOT EXISTS idx_message_reports_message_id ON message_reports (message_id);
