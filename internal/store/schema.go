package store

const schema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS owners (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  max_ledgers INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS ledgers (
  id TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL REFERENCES owners(id),
  slug TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  archive_done_after_days INTEGER,
  purge_done_after_days INTEGER,
  UNIQUE(owner_id, slug)
);

CREATE TABLE IF NOT EXISTS series (
  id TEXT PRIMARY KEY,
  ledger_id TEXT NOT NULL REFERENCES ledgers(id),
  prefix TEXT NOT NULL,
  next_n INTEGER NOT NULL DEFAULT 1,
  UNIQUE(ledger_id, prefix)
);

CREATE TABLE IF NOT EXISTS tokens (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  actor TEXT NOT NULL,
  owner_id TEXT NOT NULL REFERENCES owners(id),
  ledger_id TEXT REFERENCES ledgers(id),
  role TEXT NOT NULL DEFAULT 'write',
  email TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  ledger_id TEXT NOT NULL REFERENCES ledgers(id),
  series_id TEXT NOT NULL REFERENCES series(id),
  n INTEGER NOT NULL,
  handle TEXT NOT NULL,
  title TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  phase TEXT NOT NULL,
  size TEXT NOT NULL DEFAULT '',
  rank INTEGER NOT NULL DEFAULT 0,
  version INTEGER NOT NULL DEFAULT 1,
  pushed INTEGER NOT NULL DEFAULT 0,
  verified_at TEXT,
  since TEXT NOT NULL,
  ref TEXT NOT NULL DEFAULT '',
  gate_signoff INTEGER NOT NULL DEFAULT 0,
  gate_third INTEGER NOT NULL DEFAULT 0,
  gate_decision INTEGER NOT NULL DEFAULT 0,
  gate_time INTEGER NOT NULL DEFAULT 0,
  claimed_by TEXT,
  claimed_until TEXT,
  claim_id_hash TEXT,
  evidence TEXT,
  closed_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(ledger_id, handle),
  UNIQUE(series_id, n)
);

CREATE TABLE IF NOT EXISTS notes (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES tasks(id),
  actor TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS checks (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL REFERENCES tasks(id),
  body TEXT NOT NULL,
  done INTEGER NOT NULL DEFAULT 0,
  rank INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS deps (
  task_id TEXT NOT NULL REFERENCES tasks(id),
  depends_on TEXT NOT NULL REFERENCES tasks(id),
  PRIMARY KEY (task_id, depends_on)
);

CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ledger_id TEXT NOT NULL REFERENCES ledgers(id),
  task_id TEXT,
  actor TEXT NOT NULL,
  kind TEXT NOT NULL,
  payload TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS idempotency (
  key TEXT PRIMARY KEY,
  ledger_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_ledger_phase_rank ON tasks(ledger_id, phase, rank);
CREATE INDEX IF NOT EXISTS idx_events_ledger ON events(ledger_id, id);
CREATE INDEX IF NOT EXISTS idx_notes_task ON notes(task_id, created_at);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  actor TEXT NOT NULL DEFAULT '',
  github_id TEXT NOT NULL DEFAULT '',
  github_login TEXT NOT NULL DEFAULT '',
  owner_slug TEXT NOT NULL DEFAULT '',
  ledger_slug TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL DEFAULT '',
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS magic_links (
  id TEXT PRIMARY KEY,
  code_hash TEXT NOT NULL UNIQUE,
  token_id TEXT NOT NULL REFERENCES tokens(id),
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);
`
