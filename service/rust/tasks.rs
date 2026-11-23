use crate::contracts::{
    Create, Mutation, Reorder, ServiceError, SnapshotQuery, Task, TaskQuery, Update, canonical,
};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json, value::RawValue};
use sha2::{Digest, Sha256};
use uuid::Uuid;
use worker::{D1Database, Date, js_sys::Date as JsDate, wasm_bindgen::JsValue};

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct Principal {
    #[serde(rename = "ownerID")]
    pub owner_id: String,
    #[serde(rename = "clientID")]
    pub client_id: String,
    pub scopes: Vec<String>,
}

pub struct Tasks {
    pub db: D1Database,
    pub principal: Principal,
}

#[derive(Deserialize)]
struct Row {
    value: String,
}
#[derive(Deserialize)]
struct CursorRow<C> {
    cursor: C,
    value: String,
}
#[derive(Deserialize)]
struct Receipt {
    request_hash: String,
    result_json: String,
}
#[derive(Deserialize)]
struct SnapshotMeta {
    highwater: u64,
    timezone: String,
}
#[derive(Deserialize)]
struct Position {
    id: String,
    position: String,
}

pub fn to_raw<T: Serialize + ?Sized>(value: &T) -> Result<Box<RawValue>, ServiceError> {
    serde_json::to_string(value)
        .and_then(RawValue::from_string)
        .map_err(|_| ServiceError::internal())
}

fn check_revision(current: &Task, expected: u64, restore: bool) -> Result<(), ServiceError> {
    let deleted_ok = if restore {
        current.deleted_at.is_some()
    } else {
        current.deleted_at.is_none()
    };
    if current.revision != expected || !deleted_ok {
        return Err(ServiceError {
            status: 409,
            code: "revision_conflict",
            message: "This task changed. Review the current version before retrying.",
            current: Some(Box::new(current.clone())),
        });
    }
    Ok(())
}

impl Tasks {
    pub fn require(&self, scope: &str) -> Result<(), ServiceError> {
        if self.principal.scopes.iter().any(|s| s == scope) {
            Ok(())
        } else {
            Err(ServiceError::new(
                403,
                "insufficient_scope",
                "This action requires additional task permissions.",
            ))
        }
    }

    pub async fn get(&self, id: &str) -> Result<Task, ServiceError> {
        self.require("tasks:read")?;
        let row = self
            .db
            .prepare("SELECT value FROM task_json WHERE owner_id = ? AND id = ?")
            .bind(&[self.principal.owner_id.clone().into(), id.into()])
            .map_err(|_| ServiceError::internal())?
            .first::<Row>(None)
            .await
            .map_err(|_| ServiceError::internal())?
            .ok_or_else(|| ServiceError::new(404, "not_found", "Task not found."))?;
        serde_json::from_str(&row.value).map_err(|_| ServiceError::internal())
    }

    pub async fn list(&self) -> Result<Box<RawValue>, ServiceError> {
        self.require("tasks:read")?;
        let result = self.db.prepare(
            "SELECT j.value FROM task_json j JOIN tasks t ON t.owner_id=j.owner_id AND t.id=j.id WHERE t.owner_id=? AND t.deleted_at IS NULL ORDER BY t.position,t.id LIMIT 501"
        ).bind(&[self.principal.owner_id.clone().into()]).map_err(|_| ServiceError::internal())?
            .all().await.map_err(|_| ServiceError::internal())?;
        let rows = result
            .results::<Row>()
            .map_err(|_| ServiceError::internal())?;
        if rows.len() > 500 || rows.iter().map(|r| r.value.len()).sum::<usize>() > 1_048_576 {
            return Err(ServiceError::new(
                422,
                "result_limit",
                "The prototype task list limit was exceeded.",
            ));
        }
        let tasks = rows
            .into_iter()
            .map(|r| RawValue::from_string(r.value).map_err(|_| ServiceError::internal()))
            .collect::<Result<Vec<_>, _>>()?;
        #[derive(Serialize)]
        struct ListResponse {
            tasks: Vec<Box<RawValue>>,
        }
        to_raw(&ListResponse { tasks })
    }

    pub async fn scan(&self, input: Value) -> Result<Box<RawValue>, ServiceError> {
        self.require("tasks:read")?;
        let q = TaskQuery::parse(input)?;
        let rows=self.db.prepare("SELECT t.id AS cursor, j.value FROM task_json j JOIN tasks t ON t.owner_id=j.owner_id AND t.id=j.id WHERE t.owner_id=? AND (? OR t.deleted_at IS NULL) AND (? IS NULL OR t.status=?) AND (? IS NULL OR t.scheduled_date=?) AND (NOT ? OR t.scheduled_date IS NULL) AND (?='' OR instr(lower(t.title || char(10) || t.notes),lower(?))>0) AND t.id>? ORDER BY t.id LIMIT ?")
            .bind(&[self.principal.owner_id.clone().into(), q.include_deleted.into(),
                q.status.clone().map(JsValue::from).unwrap_or(JsValue::NULL),q.status.map(JsValue::from).unwrap_or(JsValue::NULL),
                q.scheduled_date.clone().map(JsValue::from).unwrap_or(JsValue::NULL),q.scheduled_date.map(JsValue::from).unwrap_or(JsValue::NULL),
                q.undated.into(),q.search.clone().into(),q.search.into(),q.after.into(),(q.limit+1).into()])
            .map_err(|_|ServiceError::internal())?.all().await.map_err(|_|ServiceError::internal())?;
        let (tasks, next_cursor) = Self::raw_page(
            rows.results::<CursorRow<String>>()
                .map_err(|_| ServiceError::internal())?,
            q.limit,
        )?;
        #[derive(Serialize)]
        struct TaskPage {
            tasks: Vec<Box<RawValue>>,
            next_cursor: Option<String>,
        }
        to_raw(&TaskPage { tasks, next_cursor })
    }

    async fn highwater(&self) -> Result<u64, ServiceError> {
        self.require("tasks:read")?;
        self.db
            .prepare("SELECT COALESCE(MAX(cursor),0) AS cursor FROM task_events WHERE owner_id=?")
            .bind(&[self.principal.owner_id.clone().into()])
            .map_err(|_| ServiceError::internal())?
            .first::<u64>(Some("cursor"))
            .await
            .map_err(|_| ServiceError::internal())?
            .ok_or_else(ServiceError::internal)
    }

    pub async fn snapshot(
        &self,
        input: Value,
        export: bool,
    ) -> Result<Box<RawValue>, ServiceError> {
        self.require("tasks:read")?;
        let q = SnapshotQuery::parse(input)?;
        let meta = self
            .db
            .prepare("SELECT COALESCE((SELECT MAX(cursor) FROM task_events WHERE owner_id=?),0) AS highwater, timezone FROM owners WHERE id=?")
            .bind(&[
                self.principal.owner_id.clone().into(),
                self.principal.owner_id.clone().into(),
            ])
            .map_err(|_| ServiceError::internal())?
            .first::<SnapshotMeta>(None)
            .await
            .map_err(|_| ServiceError::internal())?
            .ok_or_else(ServiceError::internal)?;
        let cursor = q.snapshot_cursor.unwrap_or(meta.highwater);
        if cursor > meta.highwater || (!q.after.is_empty() && q.snapshot_cursor.is_none()) {
            return Err(ServiceError::invalid());
        }
        // Every task version is retained. Paging at a fixed cursor reconstructs
        // one immutable board even while edits/deletes are accepted between pages.
        let rows=self.db.prepare("SELECT m.task_id AS cursor, e.after_json AS value FROM (SELECT task_id, MAX(cursor) AS max_cursor FROM task_events WHERE owner_id=? AND task_id>? AND cursor<=? GROUP BY task_id) m JOIN task_events e ON e.cursor=m.max_cursor WHERE (? OR json_extract(e.after_json,'$.deleted_at') IS NULL) ORDER BY m.task_id LIMIT ?")
            .bind(&[self.principal.owner_id.clone().into(),q.after.into(),(cursor as f64).into(),export.into(),(q.limit+1).into()])
            .map_err(|_|ServiceError::internal())?.all().await.map_err(|_|ServiceError::internal())?;
        let (tasks, next_cursor) = Self::raw_page(
            rows.results::<CursorRow<String>>()
                .map_err(|_| ServiceError::internal())?,
            q.limit,
        )?;
        #[derive(Serialize)]
        struct SnapshotPage {
            tasks: Vec<Box<RawValue>>,
            next_cursor: Option<String>,
            snapshot_cursor: u64,
            timezone: String,
            #[serde(skip_serializing_if = "Option::is_none")]
            schema_version: Option<u32>,
        }
        to_raw(&SnapshotPage {
            tasks,
            next_cursor,
            snapshot_cursor: cursor,
            timezone: meta.timezone,
            schema_version: export.then_some(1),
        })
    }

    pub async fn changes(&self, after: u64, limit: u32) -> Result<Box<RawValue>, ServiceError> {
        let highwater = self.highwater().await?;
        if after > highwater {
            return Err(ServiceError::invalid());
        }
        let rows=self.db.prepare("SELECT e.cursor AS cursor, json_object('cursor',e.cursor,'task_id',e.task_id,'operation_id',e.operation_id,'actor',e.actor,'occurred_at',e.occurred_at,'action',r.action,'before',json(e.before_json),'after',json(e.after_json)) AS value FROM task_events e JOIN operation_receipts r ON r.owner_id=e.owner_id AND r.operation_id=e.operation_id WHERE e.owner_id=? AND e.cursor>? AND e.cursor<=? ORDER BY e.cursor LIMIT ?")
            .bind(&[self.principal.owner_id.clone().into(),(after as f64).into(),(highwater as f64).into(),(limit+1).into()]).map_err(|_|ServiceError::internal())?.all().await.map_err(|_|ServiceError::internal())?;
        let (events, next_cursor) = Self::raw_page(
            rows.results::<CursorRow<u64>>()
                .map_err(|_| ServiceError::internal())?,
            limit,
        )?;
        #[derive(Serialize)]
        struct ChangesPage {
            events: Vec<Box<RawValue>>,
            next_cursor: Option<u64>,
            cursor: u64,
            highwater: u64,
        }
        to_raw(&ChangesPage {
            events,
            cursor: next_cursor.unwrap_or(highwater),
            next_cursor,
            highwater,
        })
    }

    pub async fn create(&self, input: Value) -> Result<Task, ServiceError> {
        self.require("tasks:write")?;
        let command = Create::parse(input)?;
        self.ordered(
            "create",
            &Uuid::new_v4().to_string(),
            None,
            &command.operation_id,
            serde_json::to_value(&command).map_err(|_| ServiceError::internal())?,
            command.scheduled_date.as_deref(),
            "open",
            None,
            None,
        )
        .await
    }

    pub async fn update(&self, id: &str, input: Value) -> Result<Task, ServiceError> {
        self.require("tasks:write")?;
        let command = Update::parse(input)?;
        let mut value = serde_json::to_value(&command).map_err(|_| ServiceError::internal())?;
        value["task_id"] = json!(id);
        let touches_group = command.changes.0.contains_key("scheduled_date")
            || command.changes.0.contains_key("status");
        if touches_group {
            let current = self.get(id).await?;
            let date = if let Some(date) = command.changes.0.get("scheduled_date") {
                date.as_str().map(str::to_owned)
            } else {
                current.scheduled_date.clone()
            };
            let status = command
                .changes
                .0
                .get("status")
                .and_then(Value::as_str)
                .unwrap_or(&current.status);
            if date != current.scheduled_date || status != current.status {
                return self
                    .ordered(
                        "update",
                        id,
                        Some(command.expected_revision),
                        &command.operation_id,
                        value,
                        date.as_deref(),
                        status,
                        None,
                        Some(&current),
                    )
                    .await;
            }
        }
        self.execute(
            "update",
            id,
            Some(command.expected_revision),
            &command.operation_id,
            value,
        )
        .await
    }

    pub async fn lifecycle(
        &self,
        id: &str,
        input: Value,
        restore: bool,
    ) -> Result<Task, ServiceError> {
        self.require("tasks:write")?;
        let command = Mutation::parse(input)?;
        let mut value = serde_json::to_value(&command).map_err(|_| ServiceError::internal())?;
        value["task_id"] = json!(id);
        if restore {
            let current = self.get(id).await?;
            self.ordered(
                "restore",
                id,
                Some(command.expected_revision),
                &command.operation_id,
                value,
                current.scheduled_date.as_deref(),
                &current.status,
                None,
                Some(&current),
            )
            .await
        } else {
            self.execute(
                "delete",
                id,
                Some(command.expected_revision),
                &command.operation_id,
                value,
            )
            .await
        }
    }

    pub async fn history(
        &self,
        id: &str,
        after: u64,
        limit: u32,
    ) -> Result<Box<RawValue>, ServiceError> {
        self.get(id).await?;
        let result = self.db.prepare("SELECT e.cursor AS cursor, json_object('cursor',e.cursor,'task_id',e.task_id,'operation_id',e.operation_id,'actor',e.actor,'occurred_at',e.occurred_at,'action',r.action,'before',json(e.before_json),'after',json(e.after_json)) AS value FROM task_events e JOIN operation_receipts r ON r.owner_id=e.owner_id AND r.operation_id=e.operation_id WHERE e.owner_id=? AND e.task_id=? AND e.cursor>? ORDER BY e.cursor LIMIT ?")
            .bind(&[self.principal.owner_id.clone().into(), id.into(), (after as f64).into(), (limit+1).into()])
            .map_err(|_| ServiceError::internal())?.all().await.map_err(|_| ServiceError::internal())?;
        let (events, next_cursor) = Self::raw_page(
            result
                .results::<CursorRow<u64>>()
                .map_err(|_| ServiceError::internal())?,
            limit,
        )?;
        #[derive(Serialize)]
        struct HistoryPage {
            events: Vec<Box<RawValue>>,
            next_cursor: Option<u64>,
        }
        to_raw(&HistoryPage {
            events,
            next_cursor,
        })
    }

    fn raw_page<C>(
        rows: Vec<CursorRow<C>>,
        limit: u32,
    ) -> Result<(Vec<Box<RawValue>>, Option<C>), ServiceError> {
        let mut items = Vec::with_capacity(rows.len().min(limit as usize));
        let mut size = 0;
        let mut more = false;
        let mut last_cursor = None;
        for row in rows {
            if items.len() >= limit as usize || size + row.value.len() > 900_000 {
                more = true;
                break;
            }
            size += row.value.len();
            items.push(RawValue::from_string(row.value).map_err(|_| ServiceError::internal())?);
            last_cursor = Some(row.cursor);
        }
        Ok((items, if more { last_cursor } else { None }))
    }

    pub async fn reorder(&self, id: &str, input: Value) -> Result<Task, ServiceError> {
        self.require("tasks:write")?;
        let c = Reorder::parse(input)?;
        if c.after_id.as_deref() == Some(id) || c.before_id.as_deref() == Some(id) {
            return Err(ServiceError::invalid());
        }
        let current = self.get(id).await?;
        let mut value = serde_json::to_value(&c).map_err(|_| ServiceError::internal())?;
        value["task_id"] = json!(id);
        self.ordered(
            "reorder",
            id,
            Some(c.expected_revision),
            &c.operation_id,
            value,
            current.scheduled_date.as_deref(),
            &current.status,
            Some((c.after_id.as_deref(), c.before_id.as_deref())),
            Some(&current),
        )
        .await
    }

    #[allow(clippy::too_many_arguments)]
    async fn ordered(
        &self,
        action: &str,
        id: &str,
        revision: Option<u64>,
        op: &str,
        value: Value,
        date: Option<&str>,
        status: &str,
        neighbors: Option<(Option<&str>, Option<&str>)>,
        current: Option<&Task>,
    ) -> Result<Task, ServiceError> {
        // Check the acknowledgement before reading a newer scope or compacting.
        if let Some(receipt) = self.receipt(op).await? {
            let hash = format!(
                "{:x}",
                Sha256::digest(canonical(action, &value)?.as_bytes())
            );
            return serde_json::from_value(Self::resolve_receipt(receipt, &hash)?)
                .map_err(|_| ServiceError::internal());
        }
        if let (Some(current), Some(expected)) = (current, revision) {
            check_revision(current, expected, action == "restore")?;
        }
        #[derive(Deserialize)]
        struct BoardRev {
            board_revision: u64,
        }
        #[derive(Deserialize)]
        struct LastPosition {
            position: String,
        }
        for _ in 0..8 {
            let date_val = date.map(JsValue::from).unwrap_or(JsValue::NULL);
            let board_stmt = self
                .db
                .prepare("SELECT board_revision FROM owners WHERE id=?")
                .bind(&[self.principal.owner_id.clone().into()])
                .map_err(|_| ServiceError::internal())?;
            let (board, position) = if let Some((after, before)) = neighbors {
                let scope_stmt = self
                    .db
                    .prepare("SELECT id,position FROM tasks WHERE owner_id=? AND deleted_at IS NULL AND status=? AND scheduled_date IS ? ORDER BY position,id LIMIT 10001")
                    .bind(&[self.principal.owner_id.clone().into(), status.into(), date_val.clone()])
                    .map_err(|_| ServiceError::internal())?;
                let results = self
                    .db
                    .batch(vec![board_stmt, scope_stmt])
                    .await
                    .map_err(|_| ServiceError::internal())?;
                let board = results[0]
                    .results::<BoardRev>()
                    .map_err(|_| ServiceError::internal())?
                    .into_iter()
                    .next()
                    .ok_or_else(ServiceError::internal)?
                    .board_revision;
                let scope = results[1]
                    .results::<Position>()
                    .map_err(|_| ServiceError::internal())?;
                if scope.len() > 10000 {
                    return Err(ServiceError::new(
                        422,
                        "result_limit",
                        "Ordering scope exceeds the supported limit.",
                    ));
                }
                let others: Vec<_> = scope.iter().filter(|p| p.id != id).collect();
                let index = match after {
                    Some(a) => others.iter().position(|p| p.id == a).map(|i| i + 1),
                    None => Some(0),
                }
                .ok_or_else(|| {
                    ServiceError::new(
                        409,
                        "ordering_conflict",
                        "The reviewed neighbors are no longer in this task group.",
                    )
                })?;
                if others.get(index).map(|p| p.id.as_str()) != before {
                    return Err(ServiceError::new(
                        409,
                        "ordering_conflict",
                        "The reviewed neighbors are no longer adjacent.",
                    ));
                }
                let lower = index.checked_sub(1).map(|i| others[i].position.as_str());
                let upper = others.get(index).map(|p| p.position.as_str());
                let Some(position) = crate::ordering::between(lower, upper) else {
                    if self.compact_others(id, board, &others).await? {
                        continue;
                    }
                    continue;
                };
                (board, position)
            } else {
                let last_stmt = self
                    .db
                    .prepare("SELECT position FROM tasks WHERE owner_id=? AND deleted_at IS NULL AND status=? AND scheduled_date IS ? ORDER BY position DESC,id DESC LIMIT 1")
                    .bind(&[self.principal.owner_id.clone().into(), status.into(), date_val.clone()])
                    .map_err(|_| ServiceError::internal())?;
                let results = self
                    .db
                    .batch(vec![board_stmt, last_stmt])
                    .await
                    .map_err(|_| ServiceError::internal())?;
                let board = results[0]
                    .results::<BoardRev>()
                    .map_err(|_| ServiceError::internal())?
                    .into_iter()
                    .next()
                    .ok_or_else(ServiceError::internal)?
                    .board_revision;
                let last = results[1]
                    .results::<LastPosition>()
                    .map_err(|_| ServiceError::internal())?
                    .into_iter()
                    .next();
                let lower = last.as_ref().map(|p| p.position.as_str());
                let Some(position) = crate::ordering::between(lower, None) else {
                    let scope = self
                        .db
                        .prepare("SELECT id,position FROM tasks WHERE owner_id=? AND deleted_at IS NULL AND status=? AND scheduled_date IS ? ORDER BY position,id LIMIT 10001")
                        .bind(&[self.principal.owner_id.clone().into(), status.into(), date_val])
                        .map_err(|_| ServiceError::internal())?
                        .all()
                        .await
                        .map_err(|_| ServiceError::internal())?
                        .results::<Position>()
                        .map_err(|_| ServiceError::internal())?;
                    if scope.len() > 10000 {
                        return Err(ServiceError::new(
                            422,
                            "result_limit",
                            "Ordering scope exceeds the supported limit.",
                        ));
                    }
                    let others: Vec<_> = scope.iter().filter(|p| p.id != id).collect();
                    let _ = self.compact_others(id, board, &others).await?;
                    continue;
                };
                (board, position)
            };
            let mut stored = value.clone();
            stored["_position"] = json!(position);
            stored["_board_revision"] = json!(board);
            match self
                .submit_inner(action, id, revision, op, value.clone(), stored, false)
                .await
            {
                Ok(result) => {
                    return serde_json::from_value(result).map_err(|_| ServiceError::internal());
                }
                Err(e) if e.code == "ordering_conflict" => continue,
                Err(e) => return Err(e),
            }
        }
        Err(ServiceError::new(
            409,
            "ordering_conflict",
            "The task group is busy. Read the board and retry.",
        ))
    }

    async fn compact_others(
        &self,
        id: &str,
        board: u64,
        others: &[&Position],
    ) -> Result<bool, ServiceError> {
        let positions: Vec<_> = others
            .iter()
            .enumerate()
            .map(|(i, p)| json!({"id": p.id, "position": format!("{:016x}i", (i as u64 + 1) * 1_048_576)}))
            .collect();
        let compact = json!({"positions": positions, "_board_revision": board});
        match self
            .submit_inner(
                "compact",
                id,
                None,
                &Uuid::new_v4().to_string(),
                compact.clone(),
                compact,
                false,
            )
            .await
        {
            Ok(_) => Ok(true),
            Err(e) if e.code == "ordering_conflict" => Ok(false),
            Err(e) => Err(e),
        }
    }

    async fn receipt(&self, operation_id: &str) -> Result<Option<Receipt>, ServiceError> {
        self.db.prepare("SELECT request_hash,result_json FROM operation_receipts WHERE owner_id=? AND operation_id=?")
            .bind(&[self.principal.owner_id.clone().into(), operation_id.into()]).map_err(|_| ServiceError::internal())?
            .first::<Receipt>(None).await.map_err(|_| ServiceError::internal())
    }

    fn resolve_receipt(receipt: Receipt, hash: &str) -> Result<Value, ServiceError> {
        if receipt.request_hash != hash {
            return Err(ServiceError::new(
                409,
                "operation_id_reused",
                "Use a new operation ID for a different request.",
            ));
        }
        serde_json::from_str(&receipt.result_json).map_err(|_| ServiceError::internal())
    }

    async fn execute(
        &self,
        action: &str,
        id: &str,
        revision: Option<u64>,
        operation_id: &str,
        command: Value,
    ) -> Result<Task, ServiceError> {
        let result = self
            .submit(action, id, revision, operation_id, command.clone(), command)
            .await?;
        serde_json::from_value(result).map_err(|_| ServiceError::internal())
    }

    pub(crate) async fn submit(
        &self,
        action: &str,
        id: &str,
        revision: Option<u64>,
        operation_id: &str,
        command: Value,
        stored_command: Value,
    ) -> Result<Value, ServiceError> {
        self.submit_inner(
            action,
            id,
            revision,
            operation_id,
            command,
            stored_command,
            true,
        )
        .await
    }

    #[allow(clippy::too_many_arguments)]
    async fn submit_inner(
        &self,
        action: &str,
        id: &str,
        revision: Option<u64>,
        operation_id: &str,
        command: Value,
        stored_command: Value,
        check_receipt: bool,
    ) -> Result<Value, ServiceError> {
        let hash = format!(
            "{:x}",
            Sha256::digest(canonical(action, &command)?.as_bytes())
        );
        if check_receipt && let Some(receipt) = self.receipt(operation_id).await? {
            return Self::resolve_receipt(receipt, &hash);
        }
        let timestamp: JsDate = Date::now().into();
        let timestamp: String = timestamp.to_iso_string().into();
        let result = self.db.prepare(
            "INSERT INTO operation_receipts(owner_id,operation_id,request_hash,task_id,action,expected_revision,command_json,actor,accepted_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(owner_id,operation_id) DO NOTHING"
        ).bind(&[
            self.principal.owner_id.clone().into(), operation_id.into(), hash.clone().into(),
            id.into(), action.into(), revision.map(|r| JsValue::from_f64(r as f64)).unwrap_or(JsValue::NULL),
            stored_command.to_string().into(), self.principal.client_id.clone().into(), timestamp.into()
        ]).map_err(|_| ServiceError::internal())?.run().await;
        if let Err(error) = result {
            let message = match &error {
                worker::Error::D1(error) => error.cause(),
                _ => error.to_string(),
            };
            if message.contains("operation_id_reused") {
                return Err(ServiceError::new(
                    409,
                    "operation_id_reused",
                    "Use a new operation ID for a different request.",
                ));
            }
            if message.contains("import_conflict") {
                return Err(ServiceError::new(
                    409,
                    "import_conflict",
                    "Import is incomplete or task IDs already exist.",
                ));
            }
            if message.contains("ordering_conflict") {
                return Err(ServiceError::new(
                    409,
                    "ordering_conflict",
                    "The task order changed. Read the board and retry.",
                ));
            }
            if message.contains("revision_conflict") {
                return Err(ServiceError {
                    status: 409,
                    code: "revision_conflict",
                    message: "This task changed. Review the current version before retrying.",
                    current: Some(Box::new(self.get(id).await?)),
                });
            }
            return Err(ServiceError::internal());
        }
        Self::resolve_receipt(
            self.receipt(operation_id)
                .await?
                .ok_or_else(ServiceError::internal)?,
            &hash,
        )
    }
}
