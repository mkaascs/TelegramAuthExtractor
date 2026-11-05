ALTER TABLE results RENAME COLUMN auth_key TO auth_key_encrypted;

ALTER TABLE results
    ALTER COLUMN auth_key_encrypted TYPE TEXT USING encode(auth_key_encrypted, 'base64');

ALTER TABLE sessions ADD COLUMN recipient TEXT NOT NULL DEFAULT '';
