package conversation

const activitySchema = `
ALTER TABLE conversation ADD COLUMN next_activity_position INTEGER NOT NULL DEFAULT 1;
ALTER TABLE conversation ADD COLUMN reconcile_required INTEGER NOT NULL DEFAULT 0;
CREATE TABLE activity (
 id TEXT PRIMARY KEY,
 conversation_id TEXT NOT NULL REFERENCES conversation(id) ON DELETE CASCADE,
 provider_thread_id TEXT NOT NULL,
 run_id TEXT REFERENCES run(id) ON DELETE CASCADE,
 position INTEGER,
 kind TEXT NOT NULL, role TEXT NOT NULL, status TEXT NOT NULL,
 source TEXT NOT NULL, provider_item_id TEXT NOT NULL DEFAULT '', local_key TEXT NOT NULL,
 byte_length INTEGER NOT NULL DEFAULT 0, heading_length INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL,
 UNIQUE(conversation_id,position), UNIQUE(provider_thread_id,local_key),
 FOREIGN KEY(provider_thread_id,conversation_id) REFERENCES provider_thread(id,conversation_id) ON DELETE CASCADE
);
CREATE INDEX activity_thread_item ON activity(provider_thread_id,provider_item_id);
CREATE INDEX activity_run ON activity(run_id);
CREATE TABLE activity_chunk (
 activity_id TEXT NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
 byte_offset INTEGER NOT NULL, byte_length INTEGER NOT NULL, text BLOB NOT NULL,
 PRIMARY KEY(activity_id,byte_offset)
);
CREATE TABLE activity_heading_chunk (
 activity_id TEXT NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
 byte_offset INTEGER NOT NULL, byte_length INTEGER NOT NULL, text BLOB NOT NULL,
 PRIMARY KEY(activity_id,byte_offset)
);
CREATE TABLE activity_attachment (
 activity_id TEXT NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
 position INTEGER NOT NULL, name TEXT NOT NULL, mime_type TEXT NOT NULL,
 reference TEXT NOT NULL, size INTEGER NOT NULL, PRIMARY KEY(activity_id,position)
);
CREATE TABLE history_gap (
 id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES conversation(id) ON DELETE CASCADE,
 provider_thread_id TEXT NOT NULL REFERENCES provider_thread(id) ON DELETE CASCADE,
 reason TEXT NOT NULL, resolved INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX unresolved_gap ON history_gap(provider_thread_id,reason) WHERE resolved=0;
PRAGMA user_version=2;
`
