use chrono::NaiveDate;
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use uuid::Uuid;

#[derive(Debug, Clone, Deserialize, Serialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct Task {
    pub id: String,
    pub title: String,
    pub notes: String,
    pub status: String,
    pub scheduled_date: Option<String>,
    pub position: String,
    pub revision: u64,
    pub created_at: String,
    pub updated_at: String,
    pub completed_at: Option<String>,
    pub deleted_at: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct ServiceError {
    #[serde(skip)]
    pub status: u16,
    pub code: &'static str,
    pub message: &'static str,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub current: Option<Box<Task>>,
}

impl ServiceError {
    pub fn new(status: u16, code: &'static str, message: &'static str) -> Self {
        Self {
            status,
            code,
            message,
            current: None,
        }
    }
    pub fn invalid() -> Self {
        Self::new(400, "invalid_request", "Request fields are invalid.")
    }
    pub fn internal() -> Self {
        Self::new(
            500,
            "internal_error",
            "The service could not complete this request.",
        )
    }
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Create {
    pub operation_id: String,
    pub title: String,
    #[serde(default)]
    pub notes: String,
    #[serde(default)]
    pub scheduled_date: Option<String>,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Update {
    pub operation_id: String,
    pub expected_revision: u64,
    pub changes: Changes,
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Mutation {
    pub operation_id: String,
    pub expected_revision: u64,
}
impl Mutation {
    pub fn parse(value: Value) -> Result<Self, ServiceError> {
        let command: Self = serde_json::from_value(value).map_err(|_| ServiceError::invalid())?;
        if !valid_id(&command.operation_id)
            || command.expected_revision == 0
            || command.expected_revision > 9_007_199_254_740_991
        {
            return Err(ServiceError::invalid());
        }
        Ok(command)
    }
}

// Use an explicit map to distinguish an omitted date from JSON null without
// permissive flattening or custom double-Option deserialization.
#[derive(Debug, Deserialize, Serialize)]
#[serde(transparent)]
pub struct Changes(pub serde_json::Map<String, Value>);

fn valid_id(id: &str) -> bool {
    Uuid::parse_str(id).is_ok_and(|uuid| uuid.hyphenated().to_string().eq_ignore_ascii_case(id))
}

fn valid_date(value: &str) -> bool {
    value.len() == 10
        && NaiveDate::parse_from_str(value, "%Y-%m-%d")
            .is_ok_and(|date| date.format("%Y-%m-%d").to_string() == value)
}

fn string_len(value: &str) -> usize {
    // Match the API's established UTF-16 input bounds across languages.
    value.encode_utf16().count()
}

impl Create {
    pub fn parse(value: Value) -> Result<Self, ServiceError> {
        let mut command: Self =
            serde_json::from_value(value).map_err(|_| ServiceError::invalid())?;
        command.title = command.title.trim().to_string();
        if !valid_id(&command.operation_id)
            || command.title.is_empty()
            || string_len(&command.title) > 500
            || string_len(&command.notes) > 32_000
            || command
                .scheduled_date
                .as_ref()
                .is_some_and(|date| !valid_date(date))
        {
            return Err(ServiceError::invalid());
        }
        Ok(command)
    }
}

impl Update {
    pub fn parse(value: Value) -> Result<Self, ServiceError> {
        let mut command: Self =
            serde_json::from_value(value).map_err(|_| ServiceError::invalid())?;
        if !valid_id(&command.operation_id)
            || command.expected_revision == 0
            || command.expected_revision > 9_007_199_254_740_991
            || command.changes.0.is_empty()
        {
            return Err(ServiceError::invalid());
        }
        for (key, value) in command.changes.0.iter_mut() {
            let valid = match key.as_str() {
                "title" => {
                    if let Some(title) = value.as_str() {
                        let title = title.trim().to_string();
                        let valid = !title.is_empty() && string_len(&title) <= 500;
                        *value = Value::String(title);
                        valid
                    } else {
                        false
                    }
                }
                "notes" => value
                    .as_str()
                    .is_some_and(|notes| string_len(notes) <= 32_000),
                "status" => value
                    .as_str()
                    .is_some_and(|s| s == "open" || s == "completed"),
                "scheduled_date" => value.is_null() || value.as_str().is_some_and(valid_date),
                _ => false,
            };
            if !valid {
                return Err(ServiceError::invalid());
            }
        }
        Ok(command)
    }
}

// serde_json's sorted object map gives the same canonical representation for
// equivalent input key order. Generated task IDs are excluded for create retries.
pub fn canonical(action: &str, command: &impl Serialize) -> Result<String, ServiceError> {
    serde_json::to_string(&json!({ "action": action, "command": command }))
        .map_err(|_| ServiceError::internal())
}

#[cfg(test)]
mod tests {
    use super::*;
    const ID: &str = "5f70e8f7-a0c8-4c79-a005-24b591280f85";
    #[test]
    fn shared_fixture_preserves_dates_and_notes() {
        let task: Task =
            serde_json::from_str(include_str!("../../api/fixtures/task.json")).unwrap();
        assert_eq!(task.scheduled_date.as_deref(), Some("2020-02-29"));
        assert_eq!(task.notes, "First line\n  Keep indentation and ☕");
        assert_eq!(task.revision, 2);
    }
    #[test]
    fn validates_calendar_dates_without_rollover() {
        for date in ["2020-02-29", "2000-01-01", "2030-01-01"] {
            let c = Create::parse(
                json!({"operation_id": ID, "title": " Task ", "scheduled_date": date}),
            )
            .unwrap();
            assert_eq!(c.scheduled_date.as_deref(), Some(date));
            assert_eq!(c.title, "Task");
        }
        for date in ["2025-02-29", "2026-04-31", "2026-1-01", "tomorrow"] {
            assert!(
                Create::parse(json!({"operation_id": ID, "title": "Task", "scheduled_date": date}))
                    .is_err()
            );
        }
    }
    #[test]
    fn omitted_fields_differ_from_explicit_null() {
        let c = Update::parse(
            json!({"operation_id": ID, "expected_revision": 1, "changes": {"notes": "n"}}),
        )
        .unwrap();
        assert!(!c.changes.0.contains_key("scheduled_date"));
        let c = Update::parse(json!({"operation_id": ID, "expected_revision": 1, "changes": {"scheduled_date": null}})).unwrap();
        assert!(c.changes.0["scheduled_date"].is_null());
    }
    #[test]
    fn rejects_unknown_fields_and_invalid_revisions() {
        for changes in [
            json!({}),
            json!({"title": null}),
            json!({"status": "bad"}),
            json!({"secret": 1}),
        ] {
            assert!(
                Update::parse(
                    json!({"operation_id": ID, "expected_revision": 1, "changes": changes})
                )
                .is_err()
            );
        }
        for revision in [0_u64, 9_007_199_254_740_992] {
            assert!(Update::parse(json!({"operation_id": ID, "expected_revision": revision, "changes": {"title": "T"}})).is_err());
        }
        assert!(Create::parse(json!({"operation_id": ID, "title": "T", "extra": true})).is_err());
        assert!(Create::parse(json!({"operation_id": "not-a-uuid", "title": "T"})).is_err());
    }
    #[test]
    fn normalizes_defaults_and_object_order_for_retries() {
        let a = Create::parse(json!({"operation_id": ID, "title": " Task "})).unwrap();
        let b = Create::parse(
            json!({"scheduled_date": null, "notes": "", "title": "Task", "operation_id": ID}),
        )
        .unwrap();
        assert_eq!(
            canonical("create", &a).unwrap(),
            canonical("create", &b).unwrap()
        );
    }
}

fn page_limit() -> u32 {
    50
}
#[derive(Debug, Default, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct TaskQuery {
    pub status: Option<String>,
    pub scheduled_date: Option<String>,
    #[serde(default)]
    pub undated: bool,
    #[serde(default)]
    pub search: String,
    #[serde(default)]
    pub after: String,
    #[serde(default = "page_limit")]
    pub limit: u32,
    #[serde(default)]
    pub include_deleted: bool,
}
impl TaskQuery {
    pub fn parse(value: Value) -> Result<Self, ServiceError> {
        let query: Self = serde_json::from_value(value).map_err(|_| ServiceError::invalid())?;
        if !(1..=100).contains(&query.limit)
            || (!query.after.is_empty() && !valid_id(&query.after))
            || query
                .status
                .as_ref()
                .is_some_and(|s| !matches!(s.as_str(), "open" | "completed"))
            || query
                .scheduled_date
                .as_ref()
                .is_some_and(|s| !valid_date(s))
            || (query.undated && query.scheduled_date.is_some())
            || string_len(&query.search) > 200
        {
            return Err(ServiceError::invalid());
        }
        Ok(query)
    }
}
#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct SnapshotQuery {
    pub snapshot_cursor: Option<u64>,
    #[serde(default)]
    pub after: String,
    #[serde(default = "page_limit")]
    pub limit: u32,
}
impl SnapshotQuery {
    pub fn parse(value: Value) -> Result<Self, ServiceError> {
        let q: Self = serde_json::from_value(value).map_err(|_| ServiceError::invalid())?;
        if !(1..=100).contains(&q.limit)
            || (!q.after.is_empty() && !valid_id(&q.after))
            || q.snapshot_cursor.is_some_and(|c| c > 9_007_199_254_740_991)
        {
            return Err(ServiceError::invalid());
        }
        Ok(q)
    }
}

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Reorder {
    pub operation_id: String,
    pub expected_revision: u64,
    pub after_id: Option<String>,
    pub before_id: Option<String>,
}
impl Reorder {
    pub fn parse(value: Value) -> Result<Self, ServiceError> {
        let c: Self = serde_json::from_value(value).map_err(|_| ServiceError::invalid())?;
        Mutation::parse(
            json!({"operation_id":c.operation_id,"expected_revision":c.expected_revision}),
        )?;
        if c.after_id.as_ref().is_some_and(|id| !valid_id(id))
            || c.before_id.as_ref().is_some_and(|id| !valid_id(id))
        {
            return Err(ServiceError::invalid());
        }
        Ok(c)
    }
}

impl Task {
    pub fn validate_import(&self) -> Result<(), ServiceError> {
        if !valid_id(&self.id)
            || self.title.trim().is_empty()
            || string_len(&self.title) > 500
            || string_len(&self.notes) > 32000
            || !matches!(self.status.as_str(), "open" | "completed")
            || self.revision == 0
            || self.revision > 9_007_199_254_740_991
            || self.scheduled_date.as_ref().is_some_and(|s| !valid_date(s))
            || self.position.is_empty()
            || self.position.len() > 256
            || !self
                .position
                .bytes()
                .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-')
            || chrono::DateTime::parse_from_rfc3339(&self.created_at).is_err()
            || chrono::DateTime::parse_from_rfc3339(&self.updated_at).is_err()
            || self
                .completed_at
                .as_ref()
                .is_some_and(|s| chrono::DateTime::parse_from_rfc3339(s).is_err())
            || self
                .deleted_at
                .as_ref()
                .is_some_and(|s| chrono::DateTime::parse_from_rfc3339(s).is_err())
            || (self.status == "open" && self.completed_at.is_some())
        {
            return Err(ServiceError::invalid());
        }
        Ok(())
    }
}
