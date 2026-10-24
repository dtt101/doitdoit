//! Dense lexicographic keys with integer digit arithmetic; no floating point.
//! Keys never end in the minimum digit, so a key can be inserted in every gap.
const DIGITS: &[u8] = b"0123456789abcdefghijklmnopqrstuvwxyz";
pub fn between(lower: Option<&str>, upper: Option<&str>) -> Option<String> {
    let valid =
        |s: &str| !s.is_empty() && !s.ends_with('0') && s.bytes().all(|b| DIGITS.contains(&b));
    if lower.is_some_and(|s| !valid(s))
        || upper.is_some_and(|s| !valid(s))
        || matches!((lower,upper),(Some(a),Some(b)) if a>=b)
    {
        return None;
    }
    let a = lower.unwrap_or("").as_bytes();
    let b = upper.map(str::as_bytes);
    let mut output = Vec::new();
    let mut upper_tight = true;
    for i in 0..256 {
        let lo = a
            .get(i)
            .and_then(|v| DIGITS.iter().position(|d| d == v))
            .unwrap_or(0);
        let hi = if upper_tight {
            b.and_then(|s| s.get(i))
                .and_then(|v| DIGITS.iter().position(|d| d == v))
                .unwrap_or(35)
        } else {
            35
        };
        if lo == hi {
            output.push(DIGITS[lo]);
        } else if hi > lo + 1 {
            output.push(DIGITS[(lo + hi) / 2]);
            return String::from_utf8(output).ok();
        } else {
            output.push(DIGITS[lo]);
            upper_tight = false;
        }
    }
    None // Bounded key size; caller compacts the scope atomically and retries.
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn repeatedly_inserts_at_both_edges_and_inside_tiny_gaps() {
        let mut keys = vec![between(None, None).unwrap()];
        for n in 0..400 {
            let index = if n % 3 == 0 {
                0
            } else if n % 3 == 1 {
                keys.len()
            } else {
                keys.len() / 2
            };
            let key = between(
                index.checked_sub(1).map(|i| keys[i].as_str()),
                keys.get(index).map(String::as_str),
            )
            .unwrap();
            if index > 0 {
                assert!(keys[index - 1] < key);
            }
            if index < keys.len() {
                assert!(key < keys[index]);
            }
            assert!(!key.ends_with('0'));
            keys.insert(index, key);
        }
    }
    #[test]
    fn bounds_growth_and_requests_compaction_for_legacy_or_equal_keys() {
        assert!(between(Some("old-uuid"), None).is_none());
        assert!(between(Some("a"), Some("a")).is_none());
        assert!(between(Some("a0"), None).is_none());
        assert!(between(None, Some(&format!("{}1", "0".repeat(256)))).is_none());
    }
}
