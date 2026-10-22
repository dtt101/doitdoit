mod auth;
mod contracts;
mod http;
mod mcp;
mod tasks;

use contracts::ServiceError;
use http::ApiResult;
use serde_json::{Value, json};
use tasks::Tasks;
use worker::{Context, Env, Method, Request, Response, Result, event};

#[event(scheduled)]
pub async fn scheduled(_event: worker::ScheduledEvent, env: Env, context: worker::ScheduleContext) {
    context.wait_until(async move {
        if auth::cleanup(&env).await.is_err() {
            worker::console_error!("OAuth cleanup failed");
        }
    });
}

#[event(fetch)]
pub async fn fetch(mut request: Request, env: Env, _context: Context) -> Result<Response> {
    let origin = auth::var(&env, "SERVICE_ORIGIN").unwrap_or_default();
    let mut response = match handle(&mut request, &env, &origin).await {
        Ok(response) => response,
        Err(error) => {
            // OAuth endpoints use RFC 6749's string error field; task endpoints
            // retain the richer application error contract.
            let body = if request.path().starts_with("/oauth/") {
                json!({"error": if error.status>=500 {"server_error"} else {error.code}, "error_description":error.message})
            } else {
                json!({"error":error})
            };
            let mut response = Response::from_json(&body)?.with_status(error.status);
            if matches!(error.status, 401 | 403)
                && matches!(error.code, "unauthorized" | "insufficient_scope")
            {
                response.headers_mut().set("WWW-Authenticate",&format!("Bearer resource_metadata=\"{origin}/.well-known/oauth-protected-resource\", scope=\"tasks:read tasks:write\"{}", if error.status==403 { ", error=\"insufficient_scope\"" } else { "" }))?;
            }
            response
        }
    };
    let consent_page = request.method() == Method::Get
        && request.path() == "/authorize"
        && response.status_code() == 200;
    let consent_policy = if consent_page {
        response.headers().get("Content-Security-Policy")?
    } else {
        None
    };
    for (key, value) in [
        ("Cache-Control", "no-store"),
        ("X-Content-Type-Options", "nosniff"),
        ("X-Frame-Options", "DENY"),
        ("Referrer-Policy", "no-referrer"),
        (
            "Content-Security-Policy",
            "default-src 'none'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'",
        ),
    ] {
        response.headers_mut().set(key, value)?;
    }
    if consent_page {
        // no-referrer makes native form POSTs send Origin: null in browsers.
        // Keep the consent POST same-origin while hiding referrers from GitHub.
        response
            .headers_mut()
            .set("Referrer-Policy", "same-origin")?;
        // Preserve the policy built from the validated registered callback.
        // Browsers enforce form-action across the complete OAuth redirect chain.
        if let Some(policy) = consent_policy {
            response
                .headers_mut()
                .set("Content-Security-Policy", &policy)?;
        }
    }
    Ok(response)
}
async fn handle(request: &mut Request, env: &Env, origin: &str) -> ApiResult<Response> {
    let config = worker::Url::parse(origin).map_err(|_| ServiceError::internal())?;
    if config.origin().ascii_serialization() != origin
        || !(config.scheme() == "https"
            || (config.scheme() == "http" && config.host_str() == Some("localhost")))
        || !config.username().is_empty()
        || config.password().is_some()
    {
        return Err(ServiceError::internal());
    }
    if request
        .url()
        .map_err(|_| ServiceError::invalid())?
        .origin()
        .ascii_serialization()
        != origin
    {
        return Err(ServiceError::new(
            400,
            "invalid_origin",
            "Use the configured service origin.",
        ));
    }
    if request
        .headers()
        .get("Origin")
        .map_err(|_| ServiceError::invalid())?
        .is_some_and(|o| o != origin)
    {
        return Err(ServiceError::new(
            403,
            "invalid_origin",
            "Cross-origin browser requests are not allowed.",
        ));
    }
    let path = request.path();
    if path.starts_with("/oauth/") || matches!(path.as_str(), "/authorize" | "/callback") {
        auth::rate_limit(request, env).await?;
    }
    if path != "/mcp" && !path.starts_with("/v1/") {
        return auth::route(request, env).await;
    }
    let principal = auth::authenticate(request, env).await?;
    let tasks = Tasks {
        db: env.d1("DB").map_err(|_| ServiceError::internal())?,
        principal,
    };
    tasks.require("tasks:read")?;
    if path == "/mcp" {
        let input = if request.method() == Method::Post {
            http::content_type(request, "application/json")?;
            match serde_json::from_slice(&http::bytes(request).await?) {
                Ok(value) => Some(value),
                Err(_) => {
                    return http::json(
                        &json!({"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"Parse error"}}),
                        400,
                    );
                }
            }
        } else {
            None
        };
        if input.as_ref().is_some_and(mcp::mutates) {
            tasks.require("tasks:write")?;
        }
        return mcp::handle(request, input, &tasks).await;
    }
    let (value, status) = task_route(request, &tasks).await?;
    http::json(&value, status)
}
async fn task_route(request: &mut Request, tasks: &Tasks) -> ApiResult<(Value, u16)> {
    let path = request.path();
    let method = request.method();
    if path == "/v1/tasks" && method == Method::Get {
        return Ok((json!({"tasks":tasks.list().await?}), 200));
    }
    let id = path
        .strip_prefix("/v1/tasks/")
        .filter(|id| uuid::Uuid::parse_str(id).is_ok());
    match (path.as_str(), id, method) {
        ("/v1/tasks", _, Method::Post) => {
            tasks.require("tasks:write")?;
            Ok((
                json!({"task":tasks.create(http::body(request).await?).await?}),
                201,
            ))
        }
        (_, Some(id), Method::Get) => Ok((json!({"task":tasks.get(id).await?}), 200)),
        (_, Some(id), Method::Patch) => {
            tasks.require("tasks:write")?;
            Ok((
                json!({"task":tasks.update(id,http::body(request).await?).await?}),
                200,
            ))
        }
        ("/v1/tasks", _, _) | (_, Some(_), _) => Err(ServiceError::new(
            405,
            "method_not_allowed",
            "Unsupported task method.",
        )),
        _ => Err(ServiceError::new(404, "not_found", "Endpoint not found.")),
    }
}
