ALTER TABLE sessions DROP COLUMN recipient;

ALTER TABLE results
    ALTER COLUMN auth_key_encrypted TYPE BYTEA USING decode(auth_key_encrypted, 'base64');

ALTER TABLE results RENAME COLUMN auth_key_encrypted TO auth_key;
