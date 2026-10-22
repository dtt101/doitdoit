-- Preserve existing acknowledgements and history while extending command kinds.
-- Copy/drop children before parents so foreign keys stay enabled throughout.
ALTER TABLE owners ADD COLUMN board_revision INTEGER NOT NULL DEFAULT 0;
CREATE TABLE receipts_v2 (
  owner_id TEXT NOT NULL REFERENCES owners(id), operation_id TEXT NOT NULL,
  request_hash TEXT NOT NULL, task_id TEXT NOT NULL,
  action TEXT NOT NULL CHECK(action IN ('create','update','delete','restore','reorder','compact','import')),
  expected_revision INTEGER, command_json TEXT NOT NULL CHECK(json_valid(command_json)),
  actor TEXT NOT NULL, accepted_at TEXT NOT NULL,
  result_json TEXT CHECK(result_json IS NULL OR json_valid(result_json)),
  PRIMARY KEY(owner_id,operation_id)
) STRICT;
INSERT INTO receipts_v2 SELECT * FROM operation_receipts;
CREATE TABLE events_v2 (
  cursor INTEGER PRIMARY KEY AUTOINCREMENT, owner_id TEXT NOT NULL,
  task_id TEXT NOT NULL, operation_id TEXT NOT NULL, actor TEXT NOT NULL,
  occurred_at TEXT NOT NULL, before_json TEXT, after_json TEXT,
  UNIQUE(owner_id,operation_id,task_id),
  FOREIGN KEY(owner_id,operation_id) REFERENCES receipts_v2(owner_id,operation_id)
) STRICT;
INSERT INTO events_v2 SELECT * FROM task_events;
DROP TABLE task_events;
DROP TABLE operation_receipts;
ALTER TABLE receipts_v2 RENAME TO operation_receipts;
ALTER TABLE events_v2 RENAME TO task_events;
CREATE INDEX events_owner_cursor ON task_events(owner_id,cursor);
CREATE INDEX events_owner_task_cursor ON task_events(owner_id,task_id,cursor);

CREATE TABLE import_runs (
  owner_id TEXT NOT NULL REFERENCES owners(id), id TEXT NOT NULL,
  source_sha256 TEXT NOT NULL, expected_count INTEGER NOT NULL,
  created_at TEXT NOT NULL, committed_at TEXT,
  PRIMARY KEY(owner_id,id), UNIQUE(owner_id,source_sha256)
) STRICT;
CREATE TABLE import_items (
  owner_id TEXT NOT NULL, run_id TEXT NOT NULL, task_id TEXT NOT NULL,
  value TEXT NOT NULL CHECK(json_valid(value)),
  PRIMARY KEY(owner_id,run_id,task_id),
  FOREIGN KEY(owner_id,run_id) REFERENCES import_runs(owner_id,id)
) STRICT;
CREATE TRIGGER validate_import_item BEFORE INSERT ON import_items BEGIN
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM import_runs WHERE owner_id=NEW.owner_id AND id=NEW.run_id AND committed_at IS NULL)
    THEN RAISE(ABORT,'import_conflict') END;
  SELECT CASE WHEN EXISTS (SELECT 1 FROM import_items WHERE owner_id=NEW.owner_id AND run_id=NEW.run_id AND task_id=NEW.task_id AND value!=NEW.value)
    THEN RAISE(ABORT,'import_conflict') END;
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM import_items WHERE owner_id=NEW.owner_id AND run_id=NEW.run_id AND task_id=NEW.task_id)
    AND (SELECT count(*) FROM import_items WHERE owner_id=NEW.owner_id AND run_id=NEW.run_id)>=(SELECT expected_count FROM import_runs WHERE owner_id=NEW.owner_id AND id=NEW.run_id)
    THEN RAISE(ABORT,'import_conflict') END;
END;

CREATE TRIGGER validate_operation BEFORE INSERT ON operation_receipts BEGIN
  SELECT CASE WHEN EXISTS (
    SELECT 1 FROM operation_receipts WHERE owner_id=NEW.owner_id
      AND operation_id=NEW.operation_id AND request_hash!=NEW.request_hash
  ) THEN RAISE(ABORT,'operation_id_reused') END;
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM operation_receipts WHERE owner_id=NEW.owner_id AND operation_id=NEW.operation_id
  ) THEN
    CASE WHEN NEW.action='import' AND (
      NOT EXISTS (SELECT 1 FROM import_runs WHERE owner_id=NEW.owner_id AND id=NEW.task_id AND committed_at IS NULL
        AND expected_count=(SELECT count(*) FROM import_items WHERE owner_id=NEW.owner_id AND run_id=NEW.task_id))
      OR EXISTS (SELECT 1 FROM import_items i JOIN tasks t ON t.owner_id=i.owner_id AND t.id=i.task_id WHERE i.owner_id=NEW.owner_id AND i.run_id=NEW.task_id)
    ) THEN RAISE(ABORT,'import_conflict')
    WHEN NEW.action IN ('update','delete','restore','reorder') AND NOT EXISTS (
      SELECT 1 FROM tasks WHERE owner_id=NEW.owner_id AND id=NEW.task_id
        AND revision=NEW.expected_revision
        AND ((NEW.action='restore' AND deleted_at IS NOT NULL)
          OR (NEW.action!='restore' AND deleted_at IS NULL))
    ) THEN RAISE(ABORT,'revision_conflict')
    WHEN json_type(NEW.command_json,'$._board_revision') IS NOT NULL AND NOT EXISTS (
      SELECT 1 FROM owners WHERE id=NEW.owner_id
        AND board_revision=json_extract(NEW.command_json,'$._board_revision')
    ) THEN RAISE(ABORT,'ordering_conflict') END
  END;
END;
CREATE TRIGGER apply_operation AFTER INSERT ON operation_receipts BEGIN
  INSERT INTO task_events(owner_id,task_id,operation_id,actor,occurred_at,before_json)
    SELECT NEW.owner_id,NEW.task_id,NEW.operation_id,NEW.actor,NEW.accepted_at,
      (SELECT value FROM task_json WHERE owner_id=NEW.owner_id AND id=NEW.task_id)
    WHERE NEW.action NOT IN ('compact','import');
  INSERT INTO task_events(owner_id,task_id,operation_id,actor,occurred_at,before_json)
    SELECT NEW.owner_id,j.id,NEW.operation_id,NEW.actor,NEW.accepted_at,j.value
    FROM task_json j JOIN json_each(NEW.command_json,'$.positions') p
      ON j.id=json_extract(p.value,'$.id') WHERE j.owner_id=NEW.owner_id AND NEW.action='compact';

  INSERT INTO task_events(owner_id,task_id,operation_id,actor,occurred_at,before_json)
    SELECT NEW.owner_id,i.task_id,NEW.operation_id,NEW.actor,NEW.accepted_at,NULL
    FROM import_items i WHERE i.owner_id=NEW.owner_id AND i.run_id=NEW.task_id AND NEW.action='import';
  INSERT INTO tasks(owner_id,id,title,notes,status,scheduled_date,position,revision,created_at,updated_at,completed_at,deleted_at)
    SELECT NEW.owner_id,i.task_id,json_extract(value,'$.title'),json_extract(value,'$.notes'),
      json_extract(value,'$.status'),json_extract(value,'$.scheduled_date'),json_extract(value,'$.position'),
      json_extract(value,'$.revision'),json_extract(value,'$.created_at'),json_extract(value,'$.updated_at'),
      json_extract(value,'$.completed_at'),json_extract(value,'$.deleted_at')
    FROM import_items i WHERE i.owner_id=NEW.owner_id AND i.run_id=NEW.task_id AND NEW.action='import';
  UPDATE import_runs SET committed_at=NEW.accepted_at WHERE owner_id=NEW.owner_id AND id=NEW.task_id AND NEW.action='import';

  INSERT INTO tasks(owner_id,id,title,notes,status,scheduled_date,position,revision,created_at,updated_at)
    SELECT NEW.owner_id,NEW.task_id,json_extract(NEW.command_json,'$.title'),
      json_extract(NEW.command_json,'$.notes'),'open',json_extract(NEW.command_json,'$.scheduled_date'),
      COALESCE(json_extract(NEW.command_json,'$._position'),NEW.task_id),1,NEW.accepted_at,NEW.accepted_at
    WHERE NEW.action='create';

  UPDATE tasks SET
    title=CASE WHEN NEW.action='update' AND json_type(NEW.command_json,'$.changes.title') IS NOT NULL
      THEN json_extract(NEW.command_json,'$.changes.title') ELSE title END,
    notes=CASE WHEN NEW.action='update' AND json_type(NEW.command_json,'$.changes.notes') IS NOT NULL
      THEN json_extract(NEW.command_json,'$.changes.notes') ELSE notes END,
    scheduled_date=CASE WHEN NEW.action='update' AND json_type(NEW.command_json,'$.changes.scheduled_date') IS NOT NULL
      THEN json_extract(NEW.command_json,'$.changes.scheduled_date') ELSE scheduled_date END,
    completed_at=CASE
      WHEN NEW.action='update' AND json_extract(NEW.command_json,'$.changes.status')='completed' AND status='open' THEN NEW.accepted_at
      WHEN NEW.action='update' AND json_extract(NEW.command_json,'$.changes.status')='open' THEN NULL ELSE completed_at END,
    status=CASE WHEN NEW.action='update' THEN COALESCE(json_extract(NEW.command_json,'$.changes.status'),status) ELSE status END,
    deleted_at=CASE WHEN NEW.action='delete' THEN NEW.accepted_at WHEN NEW.action='restore' THEN NULL ELSE deleted_at END,
    position=CASE WHEN NEW.action='compact' THEN
      (SELECT json_extract(p.value,'$.position') FROM json_each(NEW.command_json,'$.positions') p WHERE json_extract(p.value,'$.id')=tasks.id)
      ELSE COALESCE(json_extract(NEW.command_json,'$._position'),position) END,
    revision=revision+1,updated_at=NEW.accepted_at
    WHERE owner_id=NEW.owner_id AND (
      (NEW.action IN ('update','delete','restore','reorder') AND id=NEW.task_id)
      OR (NEW.action='compact' AND id IN (SELECT json_extract(value,'$.id') FROM json_each(NEW.command_json,'$.positions')))
    );

  UPDATE task_events SET after_json=(SELECT value FROM task_json WHERE owner_id=NEW.owner_id AND id=task_events.task_id)
    WHERE owner_id=NEW.owner_id AND operation_id=NEW.operation_id;
  UPDATE operation_receipts SET result_json=CASE WHEN NEW.action='compact'
    THEN json_object('compacted',json_array_length(NEW.command_json,'$.positions'))
    WHEN NEW.action='import' THEN json_object('run_id',NEW.task_id,'imported',(SELECT expected_count FROM import_runs WHERE owner_id=NEW.owner_id AND id=NEW.task_id))
    ELSE (SELECT value FROM task_json WHERE owner_id=NEW.owner_id AND id=NEW.task_id) END
    WHERE owner_id=NEW.owner_id AND operation_id=NEW.operation_id;
  UPDATE owners SET board_revision=board_revision+1 WHERE id=NEW.owner_id;
END;
