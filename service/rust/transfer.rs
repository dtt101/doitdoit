//! Explicit HTTP-only administrative import. Staging is invisible to readers;
//! publication, history and the commit acknowledgement are one atomic command.
use crate::{
    contracts::{ServiceError, Task},
    http::{self, ApiResult},
    tasks::Tasks,
};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use worker::{Date, Method, Request, js_sys::Date as JsDate};
#[derive(Deserialize, Serialize)]
struct Run {
    id: String,
    source_sha256: String,
    expected_count: u32,
    staged_count: u32,
    committed_at: Option<String>,
}
fn map_error(error: worker::Error) -> ServiceError {
    if let worker::Error::D1(e) = error
        && e.cause().contains("import_conflict")
    {
        ServiceError::new(
            409,
            "import_conflict",
            "Import state, source data or task IDs conflict. Review the import report.",
        )
    } else {
        ServiceError::internal()
    }
}

async fn run(tasks: &Tasks, id: &str) -> ApiResult<Run> {
    tasks.db.prepare("SELECT r.id,r.source_sha256,r.expected_count,r.committed_at,(SELECT count(*) FROM import_items i WHERE i.owner_id=r.owner_id AND i.run_id=r.id) AS staged_count FROM import_runs r WHERE r.owner_id=? AND r.id=?")
  .bind(&[tasks.principal.owner_id.clone().into(),id.into()]).map_err(|_|ServiceError::internal())?.first(None).await.map_err(|_|ServiceError::internal())?
  .ok_or_else(||ServiceError::new(404,"not_found","Import run not found."))
}
pub async fn handle(request: &mut Request, tasks: &Tasks) -> ApiResult<Value> {
    tasks.require("tasks:write")?;
    let path = request.path();
    let method = request.method();
    if path == "/v1/imports" && method == Method::Post {
        #[derive(Deserialize)]
        #[serde(deny_unknown_fields)]
        struct Begin {
            operation_id: String,
            source_sha256: String,
            expected_count: u32,
        }
        let c: Begin = serde_json::from_value(http::body(request).await?)
            .map_err(|_| ServiceError::invalid())?;
        if uuid::Uuid::parse_str(&c.operation_id).is_err()
            || c.source_sha256.len() != 64
            || !c.source_sha256.bytes().all(|b| b.is_ascii_hexdigit())
            || c.expected_count > 10000
        {
            return Err(ServiceError::invalid());
        }
        let now: JsDate = Date::now().into();
        let now: String = now.to_iso_string().into();
        tasks.db.prepare("INSERT INTO import_runs(owner_id,id,source_sha256,expected_count,created_at) VALUES(?,?,?,?,?) ON CONFLICT DO NOTHING")
   .bind(&[tasks.principal.owner_id.clone().into(),c.operation_id.clone().into(),c.source_sha256.clone().into(),c.expected_count.into(),now.into()]).map_err(|_|ServiceError::internal())?.run().await.map_err(map_error)?;
        let id = tasks
            .db
            .prepare("SELECT id FROM import_runs WHERE owner_id=? AND source_sha256=?")
            .bind(&[
                tasks.principal.owner_id.clone().into(),
                c.source_sha256.clone().into(),
            ])
            .map_err(|_| ServiceError::internal())?
            .first::<String>(Some("id"))
            .await
            .map_err(|_| ServiceError::internal())?
            .ok_or_else(ServiceError::invalid)?;
        let r = run(tasks, &id).await?;
        if r.expected_count != c.expected_count {
            return Err(ServiceError::invalid());
        }
        return Ok(json!({"run":r}));
    }
    let tail = path
        .strip_prefix("/v1/imports/")
        .ok_or_else(ServiceError::invalid)?;
    let (id, action) = tail.split_once('/').unwrap_or((tail, ""));
    if uuid::Uuid::parse_str(id).is_err() {
        return Err(ServiceError::invalid());
    }
    let _ = run(tasks, id).await?;
    match (action, method) {
        ("", Method::Get) => Ok(json!({"run":run(tasks,id).await?})),
        ("tasks", Method::Post) => {
            #[derive(Deserialize)]
            #[serde(deny_unknown_fields)]
            struct Batch {
                tasks: Vec<Task>,
            }
            let batch: Batch = serde_json::from_value(http::body(request).await?)
                .map_err(|_| ServiceError::invalid())?;
            // Leave room for auth/owner/run reads below the Free-plan per-call
            // query budget even when each staged statement is counted separately.
            if batch.tasks.is_empty() || batch.tasks.len() > 25 {
                return Err(ServiceError::invalid());
            }
            let mut statements = Vec::new();
            for task in batch.tasks {
                task.validate_import()?;
                statements.push(tasks.db.prepare("INSERT INTO import_items(owner_id,run_id,task_id,value) VALUES(?,?,?,?) ON CONFLICT DO NOTHING")
    .bind(&[tasks.principal.owner_id.clone().into(),id.into(),task.id.clone().into(),serde_json::to_string(&task).map_err(|_|ServiceError::internal())?.into()]).map_err(|_|ServiceError::internal())?);
            }
            tasks.db.batch(statements).await.map_err(map_error)?;
            Ok(json!({"run":run(tasks,id).await?}))
        }
        ("commit", Method::Post) => {
            let value = http::body(request).await?;
            if value.as_object().is_none_or(|m| !m.is_empty()) {
                return Err(ServiceError::invalid());
            }
            let command = json!({"run_id":id});
            match tasks
                .submit("import", id, None, id, command.clone(), command)
                .await
            {
                Ok(value) => Ok(value),
                Err(e) => Err(e),
            }
        }
        _ => Err(ServiceError::new(
            405,
            "method_not_allowed",
            "Unsupported import method.",
        )),
    }
}
