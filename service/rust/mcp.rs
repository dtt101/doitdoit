//! Stateless Streamable HTTP, JSON response mode (MCP 2025-11-25).
use crate::{
    contracts::ServiceError,
    http::{self, ApiResult},
    tasks::Tasks,
};
use serde_json::{Value, json};
use worker::{Method, Request, Response};

fn rpc_error(id: &Value, code: i32, message: &str) -> ApiResult<Response> {
    http::json(
        &json!({"jsonrpc":"2.0","id":id,"error":{"code":code,"message":message}}),
        200,
    )
}
fn schema(properties: Value, required: &[&str]) -> Value {
    json!({"type":"object","properties":properties,"required":required,"additionalProperties":false})
}
fn tools() -> Value {
    let id = json!({"type":"string","format":"uuid"});
    let date = json!({"type":["string","null"],"description":"Local calendar YYYY-MM-DD; null clears scheduling."});
    let title = json!({"type":"string","minLength":1,"maxLength":500});
    let notes = json!({"type":"string","maxLength":32000});
    let mut update = json!({"operation_id":id,"id":id,"expected_revision":{"type":"integer","minimum":1,"maximum":9007199254740991_u64}});
    update["changes"] = schema(
        json!({"title":title,"notes":notes,"status":{"type":"string","enum":["open","completed"]},"scheduled_date":date}),
        &[],
    );
    update["changes"]["minProperties"] = json!(1);
    let page = json!({"type":"integer","minimum":1,"maximum":100});
    let cursor = json!({"type":"integer","minimum":0,"maximum":9007199254740991_u64});
    let query = schema(
        json!({"status":{"type":"string","enum":["open","completed"]},"scheduled_date":{"type":"string"},"undated":{"type":"boolean"},"search":{"type":"string","maxLength":200},"after":id,"limit":page,"include_deleted":{"type":"boolean"}}),
        &[],
    );
    let snapshot = schema(
        json!({"snapshot_cursor":cursor,"after":id,"limit":page}),
        &[],
    );
    let mutation = schema(
        json!({"id":id,"operation_id":id,"expected_revision":{"type":"integer","minimum":1,"maximum":9007199254740991_u64}}),
        &["id", "operation_id", "expected_revision"],
    );
    json!([
        {"name":"list_tasks","description":"List tasks without changing scheduled dates, including overdue tasks.","inputSchema":query,"annotations":{"readOnlyHint":true}},
        {"name":"get_task","description":"Read a task and its current revision before editing.","inputSchema":schema(json!({"id":id}),&["id"]),"annotations":{"readOnlyHint":true}},
        {"name":"create_task","description":"Create an open task. Use a new UUID operation_id; reuse it only for identical retries.","inputSchema":schema(json!({"operation_id":id,"title":title,"notes":notes,"scheduled_date":date}),&["operation_id","title"]),"annotations":{"idempotentHint":true}},
        {"name":"update_task","description":"Edit only supplied fields at expected_revision. Date null clears scheduling. Review conflicts; never guess a winner. Identical retries reuse operation_id.","inputSchema":schema(update,&["id","operation_id","expected_revision","changes"]),"annotations":{"idempotentHint":true}},
        {"name":"delete_task","description":"Soft delete a task at its reviewed revision. The task remains recoverable; scheduling and notes are preserved.","inputSchema":mutation,"annotations":{"idempotentHint":true,"destructiveHint":true}},
        {"name":"restore_task","description":"Restore a deleted task at its reviewed revision, preserving dates, status and notes.","inputSchema":mutation,"annotations":{"idempotentHint":true}},
        {"name":"task_history","description":"Read paged task history including deleted tasks. Use next_cursor as after.","inputSchema":schema(json!({"id":id,"after":{"type":"integer","minimum":0,"maximum":9007199254740991_u64},"limit":{"type":"integer","minimum":1,"maximum":100}}),&["id"]),"annotations":{"readOnlyHint":true}},
        {"name":"task_board","description":"Read a consistent paged board. Reuse snapshot_cursor and next_cursor as after for all pages, then read changes from snapshot_cursor. Tasks are returned in ID order; group and sort by position for presentation.","inputSchema":snapshot,"annotations":{"readOnlyHint":true}},
        {"name":"task_changes","description":"Read accepted changes after a board cursor. Apply events by task revision; continue with returned cursor.","inputSchema":schema(json!({"after":cursor,"limit":page}),&[]),"annotations":{"readOnlyHint":true}},
        {"name":"export_tasks","description":"Read portable paged owner data, including recoverable deleted tasks. Keep snapshot_cursor fixed across pages.","inputSchema":snapshot,"annotations":{"readOnlyHint":true}},
        {"name":"reorder_task","description":"Move within the same date and completion group. after_id is the preceding neighbor; before_id the following neighbor. Null means a group edge. Review adjacency conflicts before retrying.","inputSchema":schema(json!({"id":id,"operation_id":id,"expected_revision":{"type":"integer","minimum":1,"maximum":9007199254740991_u64},"after_id":{"type":["string","null"],"format":"uuid"},"before_id":{"type":["string","null"],"format":"uuid"}}),&["id","operation_id","expected_revision"]),"annotations":{"idempotentHint":true}}
    ])
}
pub fn mutates(value: &Value) -> bool {
    value["method"] == "tools/call"
        && matches!(
            value["params"]["name"].as_str(),
            Some("create_task" | "update_task" | "delete_task" | "restore_task" | "reorder_task")
        )
}
pub async fn handle(request: &Request, input: Option<Value>, tasks: &Tasks) -> ApiResult<Response> {
    if request.method() != Method::Post {
        let mut r = http::json(
            &json!({"error":{"code":"method_not_allowed","message":"Use POST for stateless MCP."}}),
            405,
        )?;
        http::header(&mut r, "Allow", "POST")?;
        return Ok(r);
    }
    let accept = request
        .headers()
        .get("Accept")
        .map_err(|_| ServiceError::invalid())?
        .unwrap_or_default();
    if !accept
        .split(',')
        .any(|a| a.trim().split(';').next() == Some("application/json"))
        || !accept.contains("text/event-stream")
    {
        return http::json(
            &json!({"error":{"code":"not_acceptable","message":"Accept application/json and text/event-stream."}}),
            406,
        );
    }
    let input = input.unwrap_or(Value::Null);
    let id = input.get("id").cloned().unwrap_or(Value::Null);
    let method = input.get("method").and_then(Value::as_str);
    if !input.is_object()
        || input["jsonrpc"] != "2.0"
        || method.is_none()
        || !(id.is_null() || id.is_string() || id.is_i64() || id.is_u64())
    {
        return rpc_error(&Value::Null, -32600, "Invalid request");
    }
    if !input.get("params").is_none_or(Value::is_object) {
        return rpc_error(&id, -32602, "Invalid params");
    }
    let method = method.unwrap_or_default();
    if !input.as_object().is_some_and(|o| o.contains_key("id")) {
        // Notifications never produce a JSON-RPC response or invoke task tools.
        return Ok(Response::empty()
            .map_err(|_| ServiceError::internal())?
            .with_status(202));
    }
    if method != "initialize" {
        let version = request
            .headers()
            .get("MCP-Protocol-Version")
            .map_err(|_| ServiceError::invalid())?;
        if version
            .as_deref()
            .is_some_and(|v| !matches!(v, "2025-03-26" | "2025-06-18" | "2025-11-25"))
        {
            return Err(ServiceError::new(
                400,
                "unsupported_protocol_version",
                "Unsupported MCP protocol version.",
            ));
        }
    }
    let result = match method {
        "initialize" => {
            let params = &input["params"];
            if !params["protocolVersion"].is_string()
                || !params["capabilities"].is_object()
                || !params["clientInfo"]["name"].is_string()
                || !params["clientInfo"]["version"].is_string()
            {
                return rpc_error(&id, -32602, "Invalid initialize params");
            }
            let version = params["protocolVersion"]
                .as_str()
                .filter(|v| matches!(*v, "2025-03-26" | "2025-06-18" | "2025-11-25"))
                .unwrap_or("2025-11-25");
            json!({"protocolVersion":version,"capabilities":{"tools":{"listChanged":false}},"serverInfo":{"name":"doitdoit","version":env!("CARGO_PKG_VERSION")}})
        }
        "ping" => json!({}),
        "tools/list" => json!({"tools":tools()}),
        "tools/call" => {
            let params = &input["params"];
            let Some(name) = params["name"].as_str() else {
                return rpc_error(&id, -32602, "Invalid tool name");
            };
            let args = params.get("arguments").cloned().unwrap_or(json!({}));
            if !args.is_object() {
                return rpc_error(&id, -32602, "Invalid tool arguments");
            }
            if !matches!(
                name,
                "list_tasks"
                    | "get_task"
                    | "create_task"
                    | "update_task"
                    | "delete_task"
                    | "restore_task"
                    | "task_history"
                    | "task_board"
                    | "task_changes"
                    | "export_tasks"
                    | "reorder_task"
            ) {
                return rpc_error(&id, -32602, "Unknown tool");
            }
            let operation = call(name, args, tasks).await;
            let (result, is_error) = match operation {
                Ok(result) => (result, false),
                Err(e) => (json!({"error":e}), true),
            };
            json!({"content":[{"type":"text","text":result.to_string()}],"structuredContent":result,"isError":is_error})
        }
        _ => return rpc_error(&id, -32601, "Method not found"),
    };
    http::json(&json!({"jsonrpc":"2.0","id":id,"result":result}), 200)
}
async fn call(name: &str, mut args: Value, tasks: &Tasks) -> ApiResult<Value> {
    match name {
        "list_tasks" => {
            if args.as_object().is_some_and(|o| o.is_empty()) {
                Ok(json!({"tasks":tasks.list().await?}))
            } else {
                tasks.scan(args).await
            }
        }
        "task_board" | "export_tasks" => tasks.snapshot(args, name == "export_tasks").await,
        "task_changes" => {
            #[derive(serde::Deserialize)]
            #[serde(deny_unknown_fields)]
            struct Page {
                #[serde(default)]
                after: u64,
                #[serde(default = "default_limit")]
                limit: u32,
            }
            fn default_limit() -> u32 {
                50
            }
            let p: Page = serde_json::from_value(args).map_err(|_| ServiceError::invalid())?;
            if p.after > 9_007_199_254_740_991 || !(1..=100).contains(&p.limit) {
                return Err(ServiceError::invalid());
            }
            tasks.changes(p.after, p.limit).await
        }
        "create_task" => Ok(json!({"task":tasks.create(args).await?})),
        "get_task" | "update_task" | "delete_task" | "restore_task" | "task_history"
        | "reorder_task" => {
            let id = args
                .as_object_mut()
                .and_then(|o| o.remove("id"))
                .and_then(|v| v.as_str().map(str::to_owned))
                .filter(|id| uuid::Uuid::parse_str(id).is_ok())
                .ok_or_else(ServiceError::invalid)?;
            if name == "get_task" {
                if args.as_object().is_none_or(|o| !o.is_empty()) {
                    return Err(ServiceError::invalid());
                }
                Ok(json!({"task":tasks.get(&id).await?}))
            } else if name == "task_history" {
                #[derive(serde::Deserialize)]
                #[serde(deny_unknown_fields)]
                struct Page {
                    #[serde(default)]
                    after: u64,
                    #[serde(default = "default_limit")]
                    limit: u32,
                }
                fn default_limit() -> u32 {
                    50
                }
                let page: Page =
                    serde_json::from_value(args).map_err(|_| ServiceError::invalid())?;
                if page.after > 9_007_199_254_740_991 || !(1..=100).contains(&page.limit) {
                    return Err(ServiceError::invalid());
                }
                tasks.history(&id, page.after, page.limit).await
            } else if name == "reorder_task" {
                Ok(json!({"task":tasks.reorder(&id,args).await?}))
            } else if name == "update_task" {
                Ok(json!({"task":tasks.update(&id,args).await?}))
            } else {
                Ok(json!({"task":tasks.lifecycle(&id,args,name=="restore_task").await?}))
            }
        }
        _ => Err(ServiceError::invalid()),
    }
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn tools_expose_revision_and_nullable_date() {
        let tools = tools();
        assert_eq!(tools.as_array().unwrap().len(), 11);
        assert_eq!(
            tools[3]["inputSchema"]["properties"]["changes"]["properties"]["scheduled_date"]["type"],
            json!(["string", "null"])
        );
        assert!(
            tools[3]["inputSchema"]["required"]
                .as_array()
                .unwrap()
                .contains(&json!("expected_revision"))
        );
    }
}
