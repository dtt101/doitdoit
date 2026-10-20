//! Deliberately narrow OAuth server: public registered clients, authorization
//! code + S256 PKCE, opaque audience-bound tokens, rotating refresh credentials.
use crate::{
    contracts::ServiceError,
    http::{self, ApiResult},
    tasks::Principal,
};
use base64::{Engine, engine::general_purpose::URL_SAFE_NO_PAD};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use std::collections::BTreeMap;
use worker::{
    D1Database, Env, Fetch, Headers, Method, Request, RequestInit, Response, Url,
    wasm_bindgen::JsValue,
};

type Params = BTreeMap<String, String>;
const SCOPES: [&str; 3] = ["tasks:read", "tasks:write", "offline_access"];
pub fn digest(value: &str) -> String {
    format!("{:x}", Sha256::digest(value.as_bytes()))
}
pub fn challenge(value: &str) -> String {
    URL_SAFE_NO_PAD.encode(Sha256::digest(value.as_bytes()))
}
fn secret() -> String {
    format!(
        "{}{}",
        uuid::Uuid::new_v4().simple(),
        uuid::Uuid::new_v4().simple()
    )
}
pub fn now() -> f64 {
    worker::Date::now().as_millis() as f64 / 1000.0
}
pub fn var(env: &Env, key: &str) -> ApiResult<String> {
    Ok(env
        .var(key)
        .map_err(|_| ServiceError::internal())?
        .to_string())
}
fn db(env: &Env) -> ApiResult<D1Database> {
    env.d1("DB").map_err(|_| ServiceError::internal())
}

pub async fn rate_limit(request: &Request, env: &Env) -> ApiResult<()> {
    let binding = if request.path() == "/oauth/register" {
        "REGISTER_LIMIT"
    } else {
        "AUTH_LIMIT"
    };
    // Cloudflare supplies this header at ingress. Never trust X-Forwarded-For.
    let ip = request
        .headers()
        .get("CF-Connecting-IP")
        .map_err(|_| ServiceError::internal())?
        .unwrap_or_else(|| "local".into());
    let allowed = env
        .rate_limiter(binding)
        .map_err(|_| ServiceError::internal())?
        .limit(digest(&ip))
        .await
        .map_err(|_| ServiceError::internal())?;
    if !allowed.success {
        return Err(ServiceError::new(
            429,
            "rate_limited",
            "Too many sign-in requests. Try again in a minute.",
        ));
    }
    Ok(())
}

pub async fn cleanup(env: &Env) -> ApiResult<()> {
    let db = db(env)?;
    let now = now();
    // Keep spent credentials while a grant is live, so replay still revokes
    // its successors. Task history and operation receipts are never cleaned here.
    let statements = [
        "DELETE FROM oauth_pending WHERE digest IN (SELECT digest FROM oauth_pending WHERE expires<=? LIMIT 1000)",
        "DELETE FROM oauth_clients WHERE id IN (SELECT id FROM oauth_clients WHERE expires<=? LIMIT 1000)",
        "DELETE FROM oauth_credentials WHERE grant_id IN (SELECT g.id FROM oauth_grants g WHERE NOT EXISTS (SELECT 1 FROM oauth_credentials c WHERE c.grant_id=g.id AND c.kind IN ('code','access','refresh') AND c.expires>?) LIMIT 1000)",
        "DELETE FROM oauth_grants WHERE id IN (SELECT g.id FROM oauth_grants g WHERE NOT EXISTS (SELECT 1 FROM oauth_credentials c WHERE c.grant_id=g.id) AND ? IS NOT NULL LIMIT 1000)",
    ].into_iter().map(|sql| db.prepare(sql).bind(&[now.into()]))
        .collect::<worker::Result<Vec<_>>>().map_err(|_| ServiceError::internal())?;
    db.batch(statements)
        .await
        .map_err(|_| ServiceError::internal())?;
    Ok(())
}
fn invalid() -> ServiceError {
    ServiceError::new(
        400,
        "invalid_request",
        "Authorization expired or was invalid. Start again.",
    )
}
fn unauthorized() -> ServiceError {
    ServiceError::new(401, "unauthorized", "Authentication is required.")
}
fn grant_error() -> ServiceError {
    ServiceError::new(
        400,
        "invalid_grant",
        "The credential is invalid, expired or already used.",
    )
}
fn params(encoded: &str) -> ApiResult<Params> {
    let url = Url::parse(&format!("https://form.invalid/?{encoded}")).map_err(|_| invalid())?;
    let mut result = Params::new();
    for (key, value) in url.query_pairs() {
        if result
            .insert(key.into_owned(), value.into_owned())
            .is_some()
        {
            return Err(invalid());
        }
    }
    Ok(result)
}
fn required<'a>(p: &'a Params, key: &str) -> ApiResult<&'a str> {
    p.get(key)
        .filter(|v| !v.is_empty())
        .map(String::as_str)
        .ok_or_else(invalid)
}
async fn form(request: &mut Request) -> ApiResult<Params> {
    http::content_type(request, "application/x-www-form-urlencoded")?;
    params(std::str::from_utf8(&http::bytes(request).await?).map_err(|_| invalid())?)
}
fn scope(value: &str) -> ApiResult<String> {
    let mut scopes: Vec<_> = value.split_whitespace().collect();
    if scopes.is_empty() || scopes.iter().any(|s| !SCOPES.contains(s)) {
        return Err(ServiceError::new(
            400,
            "invalid_scope",
            "Unsupported scope.",
        ));
    }
    scopes.sort_unstable();
    scopes.dedup();
    Ok(scopes.join(" "))
}
fn redirect_valid(value: &str) -> bool {
    Url::parse(value).is_ok_and(|u| {
        u.fragment().is_none()
            && u.username().is_empty()
            && u.password().is_none()
            && u.host_str().is_some()
            && (u.scheme() == "https"
                || (u.scheme() == "http"
                    && matches!(u.host_str(), Some("localhost" | "127.0.0.1" | "[::1]"))))
    })
}
fn verifier_valid(value: &str) -> bool {
    (43..=128).contains(&value.len())
        && value
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b"-._~".contains(&b))
}
fn escape(value: &str) -> String {
    value
        .chars()
        .map(|c| match c {
            '&' | '<' | '>' | '\'' | '"' => format!("&#{};", c as u32),
            _ => c.to_string(),
        })
        .collect()
}
#[derive(Deserialize, Serialize)]
struct Client {
    client_name: String,
    redirect_uris: Vec<String>,
}
#[derive(Deserialize)]
struct Stored {
    payload: String,
}
#[derive(Serialize, Deserialize)]
struct Login {
    client: String,
    redirect: String,
    scope: String,
    state: String,
    challenge: String,
    resource: String,
    verifier: String,
}
#[derive(Deserialize)]
struct Credential {
    kind: String,
    grant_id: String,
    client: String,
    owner: String,
    scope: String,
    resource: String,
    revoked: u32,
    expires: f64,
}

async fn store_pending(env: &Env, kind: &str, browser: &str, payload: &Login) -> ApiResult<String> {
    let token = secret();
    db(env)?
        .prepare("INSERT INTO oauth_pending(digest,kind,browser,payload,expires) VALUES(?,?,?,?,?)")
        .bind(&[
            digest(&token).into(),
            kind.into(),
            digest(browser).into(),
            serde_json::to_string(payload)
                .map_err(|_| ServiceError::internal())?
                .into(),
            (now() + 600.0).into(),
        ])
        .map_err(|_| ServiceError::internal())?
        .run()
        .await
        .map_err(|_| ServiceError::internal())?;
    Ok(token)
}
fn browser(request: &Request) -> ApiResult<String> {
    request
        .headers()
        .get("Cookie")
        .map_err(|_| invalid())?
        .unwrap_or_default()
        .split(';')
        .find_map(|part| {
            part.trim()
                .strip_prefix("doitdoit_login=")
                .map(str::to_string)
        })
        .ok_or_else(invalid)
}
async fn consume_pending(env: &Env, kind: &str, token: &str, browser: &str) -> ApiResult<Login> {
    let row = db(env)?.prepare("DELETE FROM oauth_pending WHERE digest=? AND kind=? AND browser=? AND expires>? RETURNING payload")
        .bind(&[digest(token).into(),kind.into(),digest(browser).into(),now().into()]).map_err(|_| ServiceError::internal())?
        .first::<Stored>(None).await.map_err(|_| ServiceError::internal())?.ok_or_else(invalid)?;
    serde_json::from_str(&row.payload).map_err(|_| ServiceError::internal())
}
fn cookie(response: &mut Response, env: &Env, browser: &str) -> ApiResult<()> {
    let secure = if var(env, "SERVICE_ORIGIN")?.starts_with("https:") {
        "; Secure"
    } else {
        ""
    };
    http::header(
        response,
        "Set-Cookie",
        &format!("doitdoit_login={browser}; Path=/; HttpOnly; SameSite=Lax; Max-Age=600{secure}"),
    )
}
fn callback_url(login: &Login, key: &str, value: &str) -> ApiResult<String> {
    let mut url = Url::parse(&login.redirect).map_err(|_| invalid())?;
    url.query_pairs_mut()
        .append_pair(key, value)
        .append_pair("state", &login.state);
    Ok(url.to_string())
}
async fn credential(env: &Env, token: &str) -> ApiResult<Option<Credential>> {
    db(env)?.prepare("SELECT c.kind,c.grant_id,c.expires,g.client,g.owner,g.scope,g.resource,g.revoked FROM oauth_credentials c JOIN oauth_grants g ON c.grant_id=g.id WHERE c.digest=?")
        .bind(&[digest(token).into()]).map_err(|_| ServiceError::internal())?.first::<Credential>(None).await.map_err(|_| ServiceError::internal())
}
pub async fn authenticate(request: &Request, env: &Env) -> ApiResult<Principal> {
    let header = request
        .headers()
        .get("Authorization")
        .map_err(|_| unauthorized())?
        .ok_or_else(unauthorized)?;
    let (scheme, token) = header.split_once(' ').ok_or_else(unauthorized)?;
    if !scheme.eq_ignore_ascii_case("bearer") || token.is_empty() || token.contains(' ') {
        return Err(unauthorized());
    }
    let c = credential(env, token).await?.ok_or_else(unauthorized)?;
    if c.kind != "access"
        || c.expires <= now()
        || c.revoked != 0
        || c.resource != var(env, "SERVICE_ORIGIN")?
    {
        return Err(unauthorized());
    }
    let owner = var(env, "GITHUB_OWNER_ID")?;
    if owner.is_empty()
        || !owner.bytes().all(|b| b.is_ascii_digit())
        || c.owner != format!("github-{owner}")
    {
        return Err(ServiceError::new(
            403,
            "owner_required",
            "This account is not the configured owner.",
        ));
    }
    Ok(Principal {
        owner_id: c.owner,
        client_id: c.client,
        scopes: c.scope.split_whitespace().map(str::to_owned).collect(),
    })
}

pub async fn route(request: &mut Request, env: &Env) -> ApiResult<Response> {
    let origin = var(env, "SERVICE_ORIGIN")?;
    let path = request.path();
    let method = request.method();
    if method == Method::Get {
        if path == "/health" {
            return http::json(
                &json!({"status":"ok","version":env!("CARGO_PKG_VERSION")}),
                200,
            );
        }
        if path == "/.well-known/oauth-protected-resource"
            || path == "/.well-known/oauth-protected-resource/mcp"
        {
            return http::json(
                &json!({"resource":origin,"authorization_servers":[origin],"scopes_supported":SCOPES,"bearer_methods_supported":["header"]}),
                200,
            );
        }
        if path == "/.well-known/oauth-authorization-server" {
            return http::json(
                &json!({"issuer":origin,"authorization_endpoint":format!("{origin}/authorize"),"token_endpoint":format!("{origin}/oauth/token"),"registration_endpoint":format!("{origin}/oauth/register"),"revocation_endpoint":format!("{origin}/oauth/revoke"),"response_types_supported":["code"],"grant_types_supported":["authorization_code","refresh_token"],"token_endpoint_auth_methods_supported":["none"],"code_challenge_methods_supported":["S256"],"scopes_supported":SCOPES}),
                200,
            );
        }
    }
    match (path.as_str(), method) {
        ("/oauth/register", Method::Post) => register(request, env).await,
        ("/authorize", Method::Get) => authorize(request, env).await,
        ("/authorize", Method::Post) => consent(request, env).await,
        ("/callback", Method::Get) => callback(request, env).await,
        ("/oauth/token", Method::Post) => exchange(request, env).await,
        ("/oauth/revoke", Method::Post) => revoke(request, env).await,
        _ => Err(ServiceError::new(404, "not_found", "Endpoint not found.")),
    }
}
async fn register(request: &mut Request, env: &Env) -> ApiResult<Response> {
    let metadata = http::body(request).await?;
    let client: Client = serde_json::from_value(metadata.clone()).map_err(|_| invalid())?;
    if client.client_name.is_empty()
        || client.client_name.len() > 200
        || client.redirect_uris.is_empty()
        || client.redirect_uris.len() > 10
        || client.redirect_uris.iter().any(|u| !redirect_valid(u))
        || metadata
            .get("token_endpoint_auth_method")
            .is_some_and(|m| m != "none")
        || metadata.get("grant_types").is_some_and(|g| {
            !g.as_array().is_some_and(|a| {
                a.iter()
                    .all(|v| v == "authorization_code" || v == "refresh_token")
            })
        })
        || metadata
            .get("response_types")
            .is_some_and(|r| r != &json!(["code"]))
    {
        return Err(invalid());
    }
    // Registration is public, consent is never inferred from a client name.
    // Limit outstanding registrations; production also needs edge rate limits.
    let count: f64 = db(env)?
        .prepare("SELECT count(*) FROM oauth_clients WHERE expires>?")
        .bind(&[now().into()])
        .map_err(|_| ServiceError::internal())?
        .first(Some("count(*)"))
        .await
        .map_err(|_| ServiceError::internal())?
        .unwrap_or(0.0);
    if count >= 1000.0 {
        return Err(ServiceError::new(
            429,
            "registration_limit",
            "Client registration limit reached.",
        ));
    }
    let id = uuid::Uuid::new_v4().to_string();
    db(env)?
        .prepare("INSERT INTO oauth_clients(id,metadata,expires) VALUES(?,?,?)")
        .bind(&[
            id.clone().into(),
            serde_json::to_string(&client)
                .map_err(|_| ServiceError::internal())?
                .into(),
            (now() + 2_592_000.0).into(),
        ])
        .map_err(|_| ServiceError::internal())?
        .run()
        .await
        .map_err(|_| ServiceError::internal())?;
    http::json(
        &json!({"client_id":id,"client_name":client.client_name,"redirect_uris":client.redirect_uris,"token_endpoint_auth_method":"none","grant_types":["authorization_code","refresh_token"],"response_types":["code"]}),
        201,
    )
}
async fn authorize(request: &Request, env: &Env) -> ApiResult<Response> {
    // Check configuration before presenting a login that cannot complete.
    var(env, "GITHUB_CLIENT_ID")?;
    var(env, "GITHUB_CLIENT_SECRET")?;
    var(env, "GITHUB_OWNER_ID")?;
    let p = params(request.url().map_err(|_| invalid())?.query().unwrap_or(""))?;
    let client_id = required(&p, "client_id")?;
    let row: Stored = db(env)?
        .prepare("SELECT metadata AS payload FROM oauth_clients WHERE id=? AND expires>?")
        .bind(&[client_id.into(), now().into()])
        .map_err(|_| ServiceError::internal())?
        .first(None)
        .await
        .map_err(|_| ServiceError::internal())?
        .ok_or_else(invalid)?;
    let client: Client =
        serde_json::from_str(&row.payload).map_err(|_| ServiceError::internal())?;
    let redirect = required(&p, "redirect_uri")?;
    if !client.redirect_uris.iter().any(|r| r == redirect) {
        return Err(invalid());
    }
    let mut login = Login {
        client: client_id.into(),
        redirect: redirect.into(),
        scope: scope(required(&p, "scope")?)?,
        state: p.get("state").cloned().unwrap_or_default(),
        challenge: p.get("code_challenge").cloned().unwrap_or_default(),
        resource: required(&p, "resource")?.into(),
        verifier: String::new(),
    };
    if p.get("response_type").map(String::as_str) != Some("code")
        || p.get("code_challenge_method").map(String::as_str) != Some("S256")
        || login.challenge.len() != 43
        || !login
            .challenge
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b"-_".contains(&b))
    {
        return http::redirect(&callback_url(&login, "error", "invalid_request")?);
    }
    if login.resource != var(env, "SERVICE_ORIGIN")? {
        return Err(ServiceError::new(
            400,
            "invalid_target",
            "Unsupported resource.",
        ));
    }
    login.verifier = secret();
    let browser = secret();
    let handle = store_pending(env, "consent", &browser, &login).await?;
    let html = format!(
        "<!doctype html><html lang=\"en\"><meta charset=\"utf-8\"><title>Connect doitdoit</title><h1>Connect {} to doitdoit?</h1><p>Self-registered client; name is not verified.</p><p>Return to: <strong>{}</strong></p><p>Requested permissions: {}</p><form method=\"post\" action=\"/authorize\"><input type=\"hidden\" name=\"handle\" value=\"{}\"><button name=\"decision\" value=\"approve\">Allow and sign in with GitHub</button><button name=\"decision\" value=\"deny\">Deny</button></form></html>",
        escape(&client.client_name),
        escape(redirect),
        escape(&login.scope),
        handle
    );
    let mut response = Response::from_html(html).map_err(|_| ServiceError::internal())?;
    cookie(&mut response, env, &browser)?;
    Ok(response)
}
async fn consent(request: &mut Request, env: &Env) -> ApiResult<Response> {
    let p = form(request).await?;
    let browser = browser(request)?;
    let login = consume_pending(env, "consent", required(&p, "handle")?, &browser).await?;
    if p.get("decision").map(String::as_str) != Some("approve") {
        return http::redirect(&callback_url(&login, "error", "access_denied")?);
    }
    let state = store_pending(env, "upstream", &browser, &login).await?;
    let mut url = Url::parse("https://github.com/login/oauth/authorize")
        .map_err(|_| ServiceError::internal())?;
    url.query_pairs_mut()
        .append_pair("client_id", &var(env, "GITHUB_CLIENT_ID")?)
        .append_pair(
            "redirect_uri",
            &format!("{}/callback", var(env, "SERVICE_ORIGIN")?),
        )
        .append_pair("state", &state)
        .append_pair("code_challenge", &challenge(&login.verifier))
        .append_pair("code_challenge_method", "S256");
    let mut response = http::redirect(url.as_str())?;
    cookie(&mut response, env, &browser)?;
    Ok(response)
}
async fn github(
    url: &str,
    method: Method,
    headers: &[(&str, &str)],
    body: Option<String>,
) -> ApiResult<Value> {
    let h = Headers::new();
    for (key, value) in headers {
        h.set(key, value).map_err(|_| ServiceError::internal())?;
    }
    let mut init = RequestInit::new();
    init.with_method(method)
        .with_headers(h)
        .with_redirect(worker::RequestRedirect::Manual);
    if let Some(body) = body {
        init.with_body(Some(JsValue::from_str(&body)));
    }
    let request = Request::new_with_init(url, &init).map_err(|_| ServiceError::internal())?;
    let mut response = Fetch::Request(request).send().await.map_err(|_| {
        ServiceError::new(
            502,
            "identity_unavailable",
            "Sign-in could not be completed. Start again.",
        )
    })?;
    if response.status_code() != 200 {
        return Err(ServiceError::new(
            502,
            "identity_unavailable",
            "Sign-in could not be completed. Start again.",
        ));
    }
    response.json().await.map_err(|_| ServiceError::internal())
}
async fn callback(request: &Request, env: &Env) -> ApiResult<Response> {
    let p = params(request.url().map_err(|_| invalid())?.query().unwrap_or(""))?;
    let login =
        consume_pending(env, "upstream", required(&p, "state")?, &browser(request)?).await?;
    if p.contains_key("error") {
        return http::redirect(&callback_url(&login, "error", "access_denied")?);
    }
    let mut url = Url::parse("https://form.invalid").map_err(|_| invalid())?;
    url.query_pairs_mut()
        .append_pair("client_id", &var(env, "GITHUB_CLIENT_ID")?)
        .append_pair("client_secret", &var(env, "GITHUB_CLIENT_SECRET")?)
        .append_pair("code", required(&p, "code")?)
        .append_pair(
            "redirect_uri",
            &format!("{}/callback", var(env, "SERVICE_ORIGIN")?),
        )
        .append_pair("code_verifier", &login.verifier);
    let token = github(
        "https://github.com/login/oauth/access_token",
        Method::Post,
        &[
            ("Accept", "application/json"),
            ("Content-Type", "application/x-www-form-urlencoded"),
        ],
        Some(url.query().unwrap_or("").into()),
    )
    .await?;
    let token = token
        .get("access_token")
        .and_then(Value::as_str)
        .ok_or_else(|| {
            ServiceError::new(
                502,
                "identity_unavailable",
                "Sign-in could not be completed. Start again.",
            )
        })?;
    let user = github(
        "https://api.github.com/user",
        Method::Get,
        &[
            ("Authorization", &format!("Bearer {token}")),
            ("Accept", "application/vnd.github+json"),
            ("User-Agent", "doitdoit"),
        ],
        None,
    )
    .await?;
    let owner = var(env, "GITHUB_OWNER_ID")?;
    if owner.is_empty()
        || !owner.bytes().all(|b| b.is_ascii_digit())
        || user
            .get("id")
            .and_then(Value::as_u64)
            .map(|id| id.to_string())
            .as_deref()
            != Some(&owner)
    {
        return Err(ServiceError::new(
            403,
            "owner_required",
            "This account is not the configured owner.",
        ));
    }
    let owner = format!("github-{owner}");
    let code = secret();
    let grant = secret();
    let db = db(env)?;
    db.batch(vec![
        db.prepare("INSERT INTO owners(id,timezone) VALUES(?,?) ON CONFLICT(id) DO NOTHING").bind(&[owner.clone().into(),var(env,"OWNER_TIMEZONE")?.into()]).map_err(|_| ServiceError::internal())?,
        db.prepare("INSERT INTO oauth_grants(id,owner,client,scope,resource) VALUES(?,?,?,?,?)").bind(&[grant.clone().into(),owner.into(),login.client.clone().into(),login.scope.clone().into(),login.resource.clone().into()]).map_err(|_| ServiceError::internal())?,
        db.prepare("INSERT INTO oauth_credentials(digest,kind,grant_id,expires,redirect,challenge) VALUES(?,'code',?,?,?,?)").bind(&[digest(&code).into(),grant.into(),(now()+300.0).into(),login.redirect.clone().into(),login.challenge.clone().into()]).map_err(|_| ServiceError::internal())?
    ]).await.map_err(|_| ServiceError::internal())?;
    let mut response = http::redirect(&callback_url(&login, "code", &code)?)?;
    http::header(
        &mut response,
        "Set-Cookie",
        "doitdoit_login=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0",
    )?;
    Ok(response)
}
async fn exchange(request: &mut Request, env: &Env) -> ApiResult<Response> {
    let p = form(request).await?;
    let kind = match required(&p, "grant_type")? {
        "authorization_code" => "code",
        "refresh_token" => "refresh",
        _ => {
            return Err(ServiceError::new(
                400,
                "unsupported_grant_type",
                "Unsupported grant type.",
            ));
        }
    };
    let token = required(
        &p,
        if kind == "code" {
            "code"
        } else {
            "refresh_token"
        },
    )?;
    let c = credential(env, token).await?.ok_or_else(grant_error)?;
    if c.client != required(&p, "client_id")? || c.resource != required(&p, "resource")? {
        return Err(grant_error());
    }
    if c.kind == format!("used_{kind}") {
        revoke_grant(env, &c.grant_id).await?;
        return Err(grant_error());
    }
    if c.kind != kind || c.revoked != 0 || c.expires <= now() {
        return Err(grant_error());
    }
    if p.get("scope")
        .is_some_and(|s| scope(s).ok().as_deref() != Some(&c.scope))
    {
        return Err(ServiceError::new(
            400,
            "invalid_scope",
            "Refresh cannot change the granted scopes.",
        ));
    }
    let verifier = p.get("code_verifier").map(String::as_str).unwrap_or("");
    if kind == "code" && !verifier_valid(verifier) {
        return Err(grant_error());
    }
    let access = secret();
    let refresh = c
        .scope
        .split_whitespace()
        .any(|s| s == "offline_access")
        .then(secret);
    let result=db(env)?.prepare("INSERT INTO oauth_exchanges(id,predecessor,kind,client,resource,now,redirect,challenge,access_digest,refresh_digest) VALUES(?,?,?,?,?,?,?,?,?,?)")
        .bind(&[secret().into(),digest(token).into(),kind.into(),c.client.into(),c.resource.into(),now().into(),p.get("redirect_uri").cloned().unwrap_or_default().into(),challenge(verifier).into(),digest(&access).into(),refresh.as_ref().map(|s| JsValue::from_str(&digest(s))).unwrap_or(JsValue::NULL)])
        .map_err(|_| ServiceError::internal())?.run().await;
    if let Err(error) = result {
        if let worker::Error::D1(e) = &error
            && e.cause().contains("invalid_grant")
        {
            // A concurrent exchange may have consumed it after our read.
            if let Some(latest) = credential(env, token).await?
                && latest.kind == format!("used_{kind}")
            {
                revoke_grant(env, &latest.grant_id).await?;
            }
            return Err(grant_error());
        }
        return Err(ServiceError::internal());
    }
    let mut result =
        json!({"access_token":access,"token_type":"Bearer","expires_in":900,"scope":c.scope});
    if let Some(refresh) = refresh {
        result["refresh_token"] = json!(refresh);
    }
    http::json(&result, 200)
}
async fn revoke_grant(env: &Env, id: &str) -> ApiResult<()> {
    db(env)?
        .prepare("UPDATE oauth_grants SET revoked=1 WHERE id=?")
        .bind(&[id.into()])
        .map_err(|_| ServiceError::internal())?
        .run()
        .await
        .map_err(|_| ServiceError::internal())?;
    Ok(())
}
async fn revoke(request: &mut Request, env: &Env) -> ApiResult<Response> {
    let p = form(request).await?;
    if let Some(c) = credential(env, required(&p, "token")?).await?
        && c.client == required(&p, "client_id")?
    {
        revoke_grant(env, &c.grant_id).await?;
    }
    http::json(&json!({}), 200)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn pkce_rfc7636_vector() {
        assert_eq!(
            challenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"),
            "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
        );
        assert!(verifier_valid(
            "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
        ));
        assert!(!verifier_valid("short"));
    }
    #[test]
    fn validates_redirects_and_scopes() {
        for url in [
            "https://client.example/cb",
            "http://127.0.0.1:123/cb",
            "http://localhost:123/cb",
        ] {
            assert!(redirect_valid(url));
        }
        for url in [
            "http://evil.example/cb",
            "javascript:alert(1)",
            "https://user:pass@client.example/cb",
            "https://client.example/cb#fragment",
        ] {
            assert!(!redirect_valid(url));
        }
        assert_eq!(
            scope("tasks:write tasks:read tasks:read").unwrap(),
            "tasks:read tasks:write"
        );
        assert!(scope("admin").is_err());
        assert!(params("client_id=a&client_id=b").is_err());
    }
}
