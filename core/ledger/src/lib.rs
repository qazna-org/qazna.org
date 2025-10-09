//! Qazna Ledger — minimal placeholder library.
//! This crate will host the deterministic ledger engine and ordering logic.

pub fn version() -> &'static str {
    "0.1.0"
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn version_is_non_empty() {
        assert!(!version().is_empty());
    }
}
