use crate::contracts::ServiceError;
use futures_util::StreamExt;
use serde_json::Value;
use worker::{Request, Response};

pub type ApiResult<T> = std::result::Result<T, ServiceError>;
pub fn json(value: &Value, status: u16) -> ApiResult<Response> {
    Ok(Response::from_json(value)
        .map_err(|_| ServiceError::internal())?
        .with_status(status))
}
pub fn header(response: &mut Response, name: &str, value: &str) -> ApiResult<()> {
    response
        .headers_mut()
        .set(name, value)
        .map_err(|_| ServiceError::internal())
}
pub fn redirect(location: &str) -> ApiResult<Response> {
    let mut response = Response::empty()
        .map_err(|_| ServiceError::internal())?
        .with_status(302);
    header(&mut response, "Location", location)?;
    Ok(response)
}
pub fn content_type(request: &Request, expected: &str) -> ApiResult<()> {
    if request
        .headers()
        .get("Content-Type")
        .map_err(|_| ServiceError::invalid())?
        .and_then(|v| v.split(';').next().map(|s| s.trim().to_ascii_lowercase()))
        .as_deref()
        != Some(expected)
    {
        return Err(ServiceError::new(
            415,
            "unsupported_media_type",
            "Unsupported content type.",
        ));
    }
    Ok(())
}
pub async fn bytes(request: &mut Request) -> ApiResult<Vec<u8>> {
    let mut data = Vec::new();
    if request.inner().body().is_none() {
        return Ok(data);
    }
    let mut stream = request.stream().map_err(|_| ServiceError::invalid())?;
    while let Some(chunk) = stream.next().await {
        let chunk = chunk.map_err(|_| ServiceError::invalid())?;
        let limit = if request.path().starts_with("/v1/imports/") {
            262_144
        } else {
            65_536
        };
        if data.len() + chunk.len() > limit {
            return Err(ServiceError::new(
                413,
                "request_too_large",
                "Request exceeds the endpoint body limit.",
            ));
        }
        data.extend_from_slice(&chunk);
    }
    Ok(data)
}
pub async fn body(request: &mut Request) -> ApiResult<Value> {
    content_type(request, "application/json")?;
    serde_json::from_slice(&bytes(request).await?).map_err(|_| ServiceError::invalid())
}
