ALTER TABLE system_settings
    ALTER COLUMN chat_nickname SET DEFAULT '客服';

UPDATE system_settings
SET chat_nickname = '客服'
WHERE btrim(chat_nickname) IN ('', '群主');
