use crate::contracts::{
    Create, Mutation, Reorder, ServiceError, SnapshotQuery, Task, TaskQuery, Update, canonical,
};
use serde::Deserialize;
use serde_json::{Value, json};
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
struct Receipt {
    request_hash: String,
    result_json: String,
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

    pub async fn list(&self) -> Result<Vec<Task>, ServiceError> {
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
        rows.into_iter()
            .map(|r| serde_json::from_str(&r.value).map_err(|_| ServiceError::internal()))
            .collect()
    }

    pub async fn scan(&self, input: Value) -> Result<Value, ServiceError> {
        self.require("tasks:read")?;
        let q = TaskQuery::parse(input)?;
        let rows=self.db.prepare("SELECT j.value FROM task_json j JOIN tasks t ON t.owner_id=j.owner_id AND t.id=j.id WHERE t.owner_id=? AND (? OR t.deleted_at IS NULL) AND (? IS NULL OR t.status=?) AND (? IS NULL OR t.scheduled_date=?) AND (NOT ? OR t.scheduled_date IS NULL) AND instr(lower(t.title || char(10) || t.notes),lower(?))>0 AND t.id>? ORDER BY t.id LIMIT ?")
            .bind(&[self.principal.owner_id.clone().into(), q.include_deleted.into(),
                q.status.clone().map(JsValue::from).unwrap_or(JsValue::NULL),q.status.map(JsValue::from).unwrap_or(JsValue::NULL),
                q.scheduled_date.clone().map(JsValue::from).unwrap_or(JsValue::NULL),q.scheduled_date.map(JsValue::from).unwrap_or(JsValue::NULL),
                q.undated.into(),q.search.into(),q.after.into(),(q.limit+1).into()])
            .map_err(|_|ServiceError::internal())?.all().await.map_err(|_|ServiceError::internal())?;
        Self::page(
            rows.results::<Row>()
                .map_err(|_| ServiceError::internal())?,
            q.limit,
            "tasks",
            "id",
        )
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

    pub async fn snapshot(&self, input: Value, export: bool) -> Result<Value, ServiceError> {
        let q = SnapshotQuery::parse(input)?;
        let highwater = self.highwater().await?;
        let cursor = q.snapshot_cursor.unwrap_or(highwater);
        if cursor > highwater || (!q.after.is_empty() && q.snapshot_cursor.is_none()) {
            return Err(ServiceError::invalid());
        }
        // Every task version is retained. Paging at a fixed cursor reconstructs
        // one immutable board even while edits/deletes are accepted between pages.
        let rows=self.db.prepare("SELECT e.after_json AS value FROM task_events e WHERE e.owner_id=? AND e.task_id>? AND e.cursor=(SELECT MAX(v.cursor) FROM task_events v WHERE v.owner_id=e.owner_id AND v.task_id=e.task_id AND v.cursor<=?) AND (? OR json_extract(e.after_json,'$.deleted_at') IS NULL) ORDER BY e.task_id LIMIT ?")
            .bind(&[self.principal.owner_id.clone().into(),q.after.into(),(cursor as f64).into(),export.into(),(q.limit+1).into()])
            .map_err(|_|ServiceError::internal())?.all().await.map_err(|_|ServiceError::internal())?;
        let mut page = Self::page(
            rows.results::<Row>()
                .map_err(|_| ServiceError::internal())?,
            q.limit,
            "tasks",
            "id",
        )?;
        page["snapshot_cursor"] = json!(cursor);
        page["timezone"] = json!(
            self.db
                .prepare("SELECT timezone FROM owners WHERE id=?")
                .bind(&[self.principal.owner_id.clone().into()])
                .map_err(|_| ServiceError::internal())?
                .first::<String>(Some("timezone"))
                .await
                .map_err(|_| ServiceError::internal())?
                .ok_or_else(ServiceError::internal)?
        );
        if export {
            page["schema_version"] = json!(1);
        }
        Ok(page)
    }

    pub async fn changes(&self, after: u64, limit: u32) -> Result<Value, ServiceError> {
        let highwater = self.highwater().await?;
        if after > highwater {
            return Err(ServiceError::invalid());
        }
        let rows=self.db.prepare("SELECT json_object('cursor',e.cursor,'task_id',e.task_id,'operation_id',e.operation_id,'actor',e.actor,'occurred_at',e.occurred_at,'action',r.action,'before',json(e.before_json),'after',json(e.after_json)) AS value FROM task_events e JOIN operation_receipts r ON r.owner_id=e.owner_id AND r.operation_id=e.operation_id WHERE e.owner_id=? AND e.cursor>? AND e.cursor<=? ORDER BY e.cursor LIMIT ?")
            .bind(&[self.principal.owner_id.clone().into(),(after as f64).into(),(highwater as f64).into(),(limit+1).into()]).map_err(|_|ServiceError::internal())?.all().await.map_err(|_|ServiceError::internal())?;
        let mut page = Self::page(
            rows.results::<Row>()
                .map_err(|_| ServiceError::internal())?,
            limit,
            "events",
            "cursor",
        )?;
        page["cursor"] = if page["next_cursor"].is_null() {
            json!(highwater)
        } else {
            page["next_cursor"].clone()
        };
        page["highwater"] = json!(highwater);
        Ok(page)
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
        )
        .await
    }

    pub async fn update(&self, id: &str, input: Value) -> Result<Task, ServiceError> {
        self.require("tasks:write")?;
        let command = Update::parse(input)?;
        let mut value = serde_json::to_value(&command).map_err(|_| ServiceError::internal())?;
        value["task_id"] = json!(id);
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
                )
                .await;
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

    pub async fn history(&self, id: &str, after: u64, limit: u32) -> Result<Value, ServiceError> {
        self.get(id).await?;
        let result = self.db.prepare("SELECT json_object('cursor',e.cursor,'task_id',e.task_id,'operation_id',e.operation_id,'actor',e.actor,'occurred_at',e.occurred_at,'action',r.action,'before',json(e.before_json),'after',json(e.after_json)) AS value FROM task_events e JOIN operation_receipts r ON r.owner_id=e.owner_id AND r.operation_id=e.operation_id WHERE e.owner_id=? AND e.task_id=? AND e.cursor>? ORDER BY e.cursor LIMIT ?")
            .bind(&[self.principal.owner_id.clone().into(), id.into(), (after as f64).into(), (limit+1).into()])
            .map_err(|_| ServiceError::internal())?.all().await.map_err(|_| ServiceError::internal())?;
        Self::page(
            result
                .results::<Row>()
                .map_err(|_| ServiceError::internal())?,
            limit,
            "events",
            "cursor",
        )
    }

    fn page(
        rows: Vec<Row>,
        limit: u32,
        field: &str,
        cursor_field: &str,
    ) -> Result<Value, ServiceError> {
        let mut items = Vec::new();
        let mut size = 0;
        let mut more = false;
        for row in rows {
            if items.len() >= limit as usize || size + row.value.len() > 900_000 {
                more = true;
                break;
            }
            size += row.value.len();
            items.push(
                serde_json::from_str::<Value>(&row.value).map_err(|_| ServiceError::internal())?,
            );
        }
        let next = if more {
            items
                .last()
                .map(|v| v[cursor_field].clone())
                .unwrap_or(Value::Null)
        } else {
            Value::Null
        };
        Ok(json!({field:items,"next_cursor":next}))
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
    ) -> Result<Task, ServiceError> {
        // Check the acknowledgement before reading a newer scope or compacting.
        if self.receipt(op).await?.is_some() {
            return serde_json::from_value(
                self.submit(action, id, revision, op, value.clone(), value)
                    .await?,
            )
            .map_err(|_| ServiceError::internal());
        }
        #[derive(Deserialize)]
        struct Position {
            id: String,
            position: String,
        }
        for _ in 0..8 {
            let board = self
                .db
                .prepare("SELECT board_revision FROM owners WHERE id=?")
                .bind(&[self.principal.owner_id.clone().into()])
                .map_err(|_| ServiceError::internal())?
                .first::<u64>(Some("board_revision"))
                .await
                .map_err(|_| ServiceError::internal())?
                .ok_or_else(ServiceError::internal)?;
            let scope=self.db.prepare("SELECT id,position FROM tasks WHERE owner_id=? AND deleted_at IS NULL AND status=? AND scheduled_date IS ? ORDER BY position,id LIMIT 10001")
                .bind(&[self.principal.owner_id.clone().into(),status.into(),date.map(JsValue::from).unwrap_or(JsValue::NULL)])
                .map_err(|_|ServiceError::internal())?.all().await.map_err(|_|ServiceError::internal())?.results::<Position>().map_err(|_|ServiceError::internal())?;
            if scope.len() > 10000 {
                return Err(ServiceError::new(
                    422,
                    "result_limit",
                    "Ordering scope exceeds the supported limit.",
                ));
            }
            let others: Vec<_> = scope.iter().filter(|p| p.id != id).collect();
            let (lower, upper) = if let Some((after, before)) = neighbors {
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
                (
                    index.checked_sub(1).map(|i| others[i].position.as_str()),
                    others.get(index).map(|p| p.position.as_str()),
                )
            } else {
                (others.last().map(|p| p.position.as_str()), None)
            };
            let Some(position) = crate::ordering::between(lower, upper) else {
                let positions:Vec<_>=scope.iter().enumerate().map(|(i,p)|json!({"id":p.id,"position":format!("{:016x}i",(i as u64+1)*1_048_576)})).collect();
                let compact = json!({"positions":positions,"_board_revision":board});
                match self
                    .submit(
                        "compact",
                        id,
                        None,
                        &Uuid::new_v4().to_string(),
                        compact.clone(),
                        compact,
                    )
                    .await
                {
                    Ok(_) => continue,
                    Err(e) if e.code == "ordering_conflict" => continue,
                    Err(e) => return Err(e),
                }
            };
            let mut stored = value.clone();
            stored["_position"] = json!(position);
            stored["_board_revision"] = json!(board);
            match self
                .submit(action, id, revision, op, value.clone(), stored)
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

    async fn receipt(&self, operation_id: &str) -> Result<Option<Receipt>, ServiceError> {
        self.db.prepare("SELECT request_hash,result_json FROM operation_receipts WHERE owner_id=? AND operation_id=?")
            .bind(&[self.principal.owner_id.clone().into(), operation_id.into()]).map_err(|_| ServiceError::internal())?
            .first::<Receipt>(None).await.map_err(|_| ServiceError::internal())
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
        let hash = format!(
            "{:x}",
            Sha256::digest(canonical(action, &command)?.as_bytes())
        );
        let resolve = |receipt: Receipt| -> Result<Value, ServiceError> {
            if receipt.request_hash != hash {
                return Err(ServiceError::new(
                    409,
                    "operation_id_reused",
                    "Use a new operation ID for a different request.",
                ));
            }
            serde_json::from_str(&receipt.result_json).map_err(|_| ServiceError::internal())
        };
        if let Some(receipt) = self.receipt(operation_id).await? {
            return resolve(receipt);
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
        resolve(
            self.receipt(operation_id)
                .await?
                .ok_or_else(ServiceError::internal)?,
        )
    }
}
