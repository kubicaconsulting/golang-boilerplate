CREATE TABLE users (
 id text PRIMARY KEY,
 name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
 email text NOT NULL CHECK (email = lower(btrim(email)) AND length(email) BETWEEN 3 AND 254),
 created_at timestamptz NOT NULL,
 CONSTRAINT users_email_unique UNIQUE (email)
);
CREATE INDEX users_created_at_id_idx ON users (created_at, id);
