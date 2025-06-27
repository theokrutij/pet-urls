ALTER TABLE url_tokens 
ADD COLUMN owner_id BYTEA;

UPDATE url_tokens 
SET owner_ID = ''::BYTEA;

ALTER TABLE url_tokens
ALTER COLUMN owner_id SET NOT NULL;

