use crate::contracts::{Create, ServiceError, Task, Update, canonical};
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

    pub async fn create(&self, input: Value) -> Result<Task, ServiceError> {
        self.require("tasks:write")?;
        let command = Create::parse(input)?;
        self.execute(
            "create",
            &Uuid::new_v4().to_string(),
            None,
            &command.operation_id,
            serde_json::to_value(&command).map_err(|_| ServiceError::internal())?,
        )
        .await
    }

    pub async fn update(&self, id: &str, input: Value) -> Result<Task, ServiceError> {
        self.require("tasks:write")?;
        let command = Update::parse(input)?;
        let mut value = serde_json::to_value(&command).map_err(|_| ServiceError::internal())?;
        value["task_id"] = json!(id);
        self.execute(
            "update",
            id,
            Some(command.expected_revision),
            &command.operation_id,
            value,
        )
        .await
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
        let hash = format!(
            "{:x}",
            Sha256::digest(canonical(action, &command)?.as_bytes())
        );
        let resolve = |receipt: Receipt| -> Result<Task, ServiceError> {
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
            command.to_string().into(), self.principal.client_id.clone().into(), timestamp.into()
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
