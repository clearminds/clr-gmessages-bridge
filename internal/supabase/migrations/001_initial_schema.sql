-- Google Messages Bridge — Supabase initial schema
-- Auto-applied on first bridge startup via SUPABASE_DB_URL

-- Conversations table
CREATE TABLE IF NOT EXISTS conversations (
    conversation_id TEXT PRIMARY KEY,
    name TEXT,
    last_message_time TIMESTAMPTZ,
    last_message_preview TEXT,
    is_group BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Messages table
CREATE TABLE IF NOT EXISTS messages (
    id TEXT NOT NULL,
    conversation_id TEXT NOT NULL REFERENCES conversations(conversation_id) ON DELETE CASCADE,
    sender_name TEXT,
    sender_number TEXT,
    content TEXT,
    timestamp TIMESTAMPTZ NOT NULL,
    is_from_me BOOLEAN DEFAULT FALSE,
    media_type TEXT,
    media_url TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, conversation_id)
);

-- Contacts table
CREATE TABLE IF NOT EXISTS contacts (
    number TEXT PRIMARY KEY,
    name TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for query performance
CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages(conversation_id);
CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_messages_content ON messages USING GIN (to_tsvector('english', coalesce(content, '')));
CREATE INDEX IF NOT EXISTS idx_conversations_last_message ON conversations(last_message_time DESC);
CREATE INDEX IF NOT EXISTS idx_contacts_name ON contacts(name);

-- Auto-update updated_at triggers
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'conversations_updated_at') THEN
        CREATE TRIGGER conversations_updated_at
            BEFORE UPDATE ON conversations
            FOR EACH ROW EXECUTE FUNCTION update_updated_at();
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'contacts_updated_at') THEN
        CREATE TRIGGER contacts_updated_at
            BEFORE UPDATE ON contacts
            FOR EACH ROW EXECUTE FUNCTION update_updated_at();
    END IF;
END;
$$;

-- RPC functions for PostgREST upserts (called via /rest/v1/rpc/...)
-- These preserve GREATEST/COALESCE logic that plain PostgREST upserts can't do.

CREATE OR REPLACE FUNCTION upsert_conversation(
    p_conversation_id TEXT,
    p_name TEXT,
    p_last_message_time TIMESTAMPTZ,
    p_is_group BOOLEAN,
    p_last_message_preview TEXT
) RETURNS VOID AS $$
BEGIN
    INSERT INTO conversations (conversation_id, name, last_message_time, is_group, last_message_preview)
    VALUES (p_conversation_id, p_name, p_last_message_time, p_is_group, p_last_message_preview)
    ON CONFLICT (conversation_id) DO UPDATE SET
        name = COALESCE(NULLIF(p_name, ''), conversations.name),
        last_message_time = GREATEST(conversations.last_message_time, p_last_message_time),
        is_group = p_is_group,
        last_message_preview = COALESCE(NULLIF(p_last_message_preview, ''), conversations.last_message_preview);
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION upsert_message(
    p_id TEXT,
    p_conversation_id TEXT,
    p_sender_name TEXT,
    p_sender_number TEXT,
    p_content TEXT,
    p_timestamp TIMESTAMPTZ,
    p_is_from_me BOOLEAN,
    p_media_type TEXT,
    p_media_url TEXT
) RETURNS VOID AS $$
BEGIN
    INSERT INTO messages (id, conversation_id, sender_name, sender_number, content, timestamp, is_from_me, media_type, media_url)
    VALUES (p_id, p_conversation_id, p_sender_name, p_sender_number, p_content, p_timestamp, p_is_from_me, p_media_type, p_media_url)
    ON CONFLICT (id, conversation_id) DO UPDATE SET
        content = COALESCE(NULLIF(p_content, ''), messages.content),
        media_type = COALESCE(NULLIF(p_media_type, ''), messages.media_type),
        media_url = COALESCE(NULLIF(p_media_url, ''), messages.media_url);
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION upsert_contact(
    p_number TEXT,
    p_name TEXT
) RETURNS VOID AS $$
BEGIN
    INSERT INTO contacts (number, name)
    VALUES (p_number, p_name)
    ON CONFLICT (number) DO UPDATE SET
        name = COALESCE(NULLIF(p_name, ''), contacts.name);
END;
$$ LANGUAGE plpgsql;
