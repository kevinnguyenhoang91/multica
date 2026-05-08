-- Add emoji icon to agent, mirroring the `icon` column on the `project` table.
-- NULL means "no emoji set" — the UI falls back to the Bot icon or avatar_url.
ALTER TABLE agent
    ADD COLUMN IF NOT EXISTS icon TEXT;
