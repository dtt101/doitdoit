-- Receipts are commands. Their insert trigger commits task, event and result in
-- ONE SQLite statement; a trigger failure rolls the entire statement back.
PRAGMA foreign_keys = ON;

CREATE TABLE owners (
  id TEXT PRIMARY KEY,
  timezone TEXT NOT NULL
) STRICT;

CREATE TABLE tasks (
  owner_id TEXT NOT NULL REFERENCES owners(id),
  id TEXT NOT NULL,
  title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 500),
  notes TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('open', 'completed')),
  scheduled_date TEXT,
  position TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK(revision > 0),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  completed_at TEXT,
  deleted_at TEXT,
  PRIMARY KEY(owner_id, id)
) STRICT;
CREATE INDEX tasks_board ON tasks(owner_id, deleted_at, status, scheduled_date, position, id);

CREATE TABLE operation_receipts (
  owner_id TEXT NOT NULL REFERENCES owners(id),
  operation_id TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  task_id TEXT NOT NULL,
  action TEXT NOT NULL CHECK(action IN ('create', 'update')),
  expected_revision INTEGER,
  command_json TEXT NOT NULL CHECK(json_valid(command_json)),
  actor TEXT NOT NULL,
  accepted_at TEXT NOT NULL,
  result_json TEXT CHECK(result_json IS NULL OR json_valid(result_json)),
  PRIMARY KEY(owner_id, operation_id)
) STRICT;

CREATE TABLE task_events (
  cursor INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  operation_id TEXT NOT NULL,
  actor TEXT NOT NULL,
  occurred_at TEXT NOT NULL,
  before_json TEXT,
  after_json TEXT,
  UNIQUE(owner_id, operation_id),
  FOREIGN KEY(owner_id, operation_id)
    REFERENCES operation_receipts(owner_id, operation_id)
) STRICT;
CREATE INDEX events_owner_cursor ON task_events(owner_id, cursor);

-- One canonical representation for tasks in HTTP, MCP, receipts and history.
CREATE VIEW task_json AS SELECT owner_id, id, json_object(
  'id', id, 'title', title, 'notes', notes, 'status', status,
  'scheduled_date', scheduled_date, 'position', position, 'revision', revision,
  'created_at', created_at, 'updated_at', updated_at,
  'completed_at', completed_at, 'deleted_at', deleted_at
) AS value FROM tasks;

CREATE TRIGGER validate_operation BEFORE INSERT ON operation_receipts BEGIN
  SELECT CASE WHEN EXISTS (
    SELECT 1 FROM operation_receipts
    WHERE owner_id = NEW.owner_id AND operation_id = NEW.operation_id
      AND request_hash != NEW.request_hash
  ) THEN RAISE(ABORT, 'operation_id_reused') END;

  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM operation_receipts
    WHERE owner_id = NEW.owner_id AND operation_id = NEW.operation_id
  ) AND NEW.action = 'update' AND NOT EXISTS (
    SELECT 1 FROM tasks WHERE owner_id = NEW.owner_id AND id = NEW.task_id
      AND revision = NEW.expected_revision AND deleted_at IS NULL
  ) THEN RAISE(ABORT, 'revision_conflict') END;
END;

CREATE TRIGGER apply_operation AFTER INSERT ON operation_receipts BEGIN
  INSERT INTO task_events (
    owner_id, task_id, operation_id, actor, occurred_at, before_json
  ) VALUES (
    NEW.owner_id, NEW.task_id, NEW.operation_id, NEW.actor, NEW.accepted_at,
    (SELECT value FROM task_json WHERE owner_id = NEW.owner_id AND id = NEW.task_id)
  );

  INSERT INTO tasks (
    owner_id, id, title, notes, status, scheduled_date, position, revision,
    created_at, updated_at
  ) SELECT
    NEW.owner_id, NEW.task_id,
    json_extract(NEW.command_json, '$.title'),
    json_extract(NEW.command_json, '$.notes'), 'open',
    json_extract(NEW.command_json, '$.scheduled_date'),
    NEW.task_id, 1, NEW.accepted_at, NEW.accepted_at
  WHERE NEW.action = 'create';

  UPDATE tasks SET
    title = CASE WHEN json_type(NEW.command_json, '$.changes.title') IS NULL
      THEN title ELSE json_extract(NEW.command_json, '$.changes.title') END,
    notes = CASE WHEN json_type(NEW.command_json, '$.changes.notes') IS NULL
      THEN notes ELSE json_extract(NEW.command_json, '$.changes.notes') END,
    scheduled_date = CASE WHEN json_type(NEW.command_json, '$.changes.scheduled_date') IS NULL
      THEN scheduled_date ELSE json_extract(NEW.command_json, '$.changes.scheduled_date') END,
    completed_at = CASE
      WHEN json_extract(NEW.command_json, '$.changes.status') = 'completed' AND status = 'open'
        THEN NEW.accepted_at
      WHEN json_extract(NEW.command_json, '$.changes.status') = 'open' THEN NULL
      ELSE completed_at END,
    status = COALESCE(json_extract(NEW.command_json, '$.changes.status'), status),
    revision = revision + 1,
    updated_at = NEW.accepted_at
  WHERE NEW.action = 'update' AND owner_id = NEW.owner_id AND id = NEW.task_id;

  UPDATE task_events SET after_json = (
    SELECT value FROM task_json WHERE owner_id = NEW.owner_id AND id = NEW.task_id
  ) WHERE owner_id = NEW.owner_id AND operation_id = NEW.operation_id;

  UPDATE operation_receipts SET result_json = (
    SELECT value FROM task_json WHERE owner_id = NEW.owner_id AND id = NEW.task_id
  ) WHERE owner_id = NEW.owner_id AND operation_id = NEW.operation_id;
END;
