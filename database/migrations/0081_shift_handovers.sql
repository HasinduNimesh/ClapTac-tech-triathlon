BEGIN;
CREATE TABLE shared.shift_handovers (
 id uuid PRIMARY KEY,
 role text NOT NULL CHECK(role IN ('dispatcher','store')),
 scope text NOT NULL,
 author_id text NOT NULL REFERENCES shared.users(id),
 author_name text NOT NULL,
 snapshot jsonb NOT NULL,
 published_at timestamptz NOT NULL DEFAULT now(),
 acknowledged_by text REFERENCES shared.users(id),
 acknowledged_name text,
 acknowledged_at timestamptz,
 CHECK ((role='dispatcher' AND scope='fleet') OR (role='store' AND scope<>'' AND scope<>'fleet')),
 CHECK ((acknowledged_by IS NULL AND acknowledged_name IS NULL AND acknowledged_at IS NULL)
     OR (acknowledged_by IS NOT NULL AND acknowledged_name IS NOT NULL AND acknowledged_at IS NOT NULL AND acknowledged_by<>author_id))
);
CREATE INDEX shift_handovers_history ON shared.shift_handovers(role,scope,published_at DESC,id DESC);
COMMIT;
