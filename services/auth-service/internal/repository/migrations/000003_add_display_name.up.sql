ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
UPDATE users SET display_name = tag WHERE display_name = '';
