ALTER TABLE chats ADD COLUMN chat_type TEXT NOT NULL DEFAULT 'private' CHECK (chat_type IN ('private', 'group'));
ALTER TABLE chats ADD COLUMN name TEXT;
ALTER TABLE chats ADD COLUMN created_by UUID;

ALTER TABLE chat_members ADD COLUMN role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member'));
