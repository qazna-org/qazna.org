//! Qazna Ledger — deterministic in-memory accounting engine.
//! This crate provides a thread-safe state machine used by the API layer.

use chrono::{DateTime, Utc};
use std::collections::{BTreeMap, HashMap};
use std::fmt;
use std::sync::{Arc, RwLock};
use ulid::Ulid;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Money {
    pub currency: String,
    pub amount: i64,
}

impl Money {
    pub fn new(currency: impl Into<String>, amount: i64) -> Self {
        Self {
            currency: currency.into(),
            amount,
        }
    }

    pub fn is_positive(&self) -> bool {
        self.amount > 0
    }

    pub fn is_zero(&self) -> bool {
        self.amount == 0
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Account {
    pub id: String,
    pub created_at: DateTime<Utc>,
    pub balances: BTreeMap<String, i64>,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Transaction {
    pub id: String,
    pub created_at: DateTime<Utc>,
    pub from_account_id: String,
    pub to_account_id: String,
    pub currency: String,
    pub amount: i64,
    pub idempotency_key: Option<String>,
    pub sequence: u64,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum LedgerError {
    NotFound,
    InsufficientFunds,
    InvalidAmount,
    InvalidCurrency,
}

impl fmt::Display for LedgerError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            LedgerError::NotFound => write!(f, "not found"),
            LedgerError::InsufficientFunds => write!(f, "insufficient funds"),
            LedgerError::InvalidAmount => write!(f, "invalid amount (must be > 0)"),
            LedgerError::InvalidCurrency => write!(f, "invalid currency"),
        }
    }
}

impl std::error::Error for LedgerError {}

#[derive(Clone)]
struct AccountRecord {
    created_at: DateTime<Utc>,
    balances: HashMap<String, i64>,
}

#[derive(Clone)]
struct TransactionRecord {
    id: String,
    created_at: DateTime<Utc>,
    from_account_id: String,
    to_account_id: String,
    currency: String,
    amount: i64,
    idempotency_key: Option<String>,
    sequence: u64,
}

struct State {
    accounts: HashMap<String, AccountRecord>,
    transactions: Vec<TransactionRecord>,
    idempotency: HashMap<String, TransactionRecord>,
    sequence: u64,
}

impl State {
    fn new() -> Self {
        Self {
            accounts: HashMap::new(),
            transactions: Vec::new(),
            idempotency: HashMap::new(),
            sequence: 0,
        }
    }
}

#[derive(Clone)]
pub struct Ledger {
    inner: Arc<RwLock<State>>,
}

impl Ledger {
    pub fn new() -> Self {
        Self {
            inner: Arc::new(RwLock::new(State::new())),
        }
    }

    pub fn create_account(&self, initial: Money) -> Result<Account, LedgerError> {
        if initial.currency.trim().is_empty() {
            return Err(LedgerError::InvalidCurrency);
        }
        if initial.amount < 0 {
            return Err(LedgerError::InvalidAmount);
        }

        let mut state = self.inner.write().expect("ledger lock poisoned");

        let id = Ulid::new().to_string();
        let now = Utc::now();

        let mut balances = HashMap::new();
        if initial.amount > 0 {
            balances.insert(initial.currency.clone(), initial.amount);
        }

        let record = AccountRecord {
            created_at: now,
            balances,
        };
        state.accounts.insert(id.clone(), record.clone());

        Ok(to_public_account(id, &record))
    }

    pub fn get_account(&self, id: &str) -> Result<Account, LedgerError> {
        let state = self.inner.read().expect("ledger lock poisoned");
        let record = state.accounts.get(id).ok_or(LedgerError::NotFound)?;
        Ok(to_public_account(id.to_string(), record))
    }

    pub fn get_balance(&self, id: &str, currency: &str) -> Result<Money, LedgerError> {
        let state = self.inner.read().expect("ledger lock poisoned");
        let record = state.accounts.get(id).ok_or(LedgerError::NotFound)?;
        Ok(Money {
            currency: currency.to_string(),
            amount: *record.balances.get(currency).unwrap_or(&0),
        })
    }

    pub fn transfer(
        &self,
        from_id: &str,
        to_id: &str,
        amount: Money,
        idempotency_key: Option<&str>,
    ) -> Result<Transaction, LedgerError> {
        if amount.currency.trim().is_empty() {
            return Err(LedgerError::InvalidCurrency);
        }
        if amount.amount <= 0 {
            return Err(LedgerError::InvalidAmount);
        }

        let mut state = self.inner.write().expect("ledger lock poisoned");

        if let Some(key) = idempotency_key {
            if let Some(tx) = state.idempotency.get(key) {
                return Ok(public_transaction(tx));
            }
        }

        let from = state
            .accounts
            .get_mut(from_id)
            .ok_or(LedgerError::NotFound)?;
        let to = state.accounts.get_mut(to_id).ok_or(LedgerError::NotFound)?;

        let from_balance = from.balances.entry(amount.currency.clone()).or_insert(0);
        if *from_balance < amount.amount {
            return Err(LedgerError::InsufficientFunds);
        }
        *from_balance -= amount.amount;

        let to_balance = to.balances.entry(amount.currency.clone()).or_insert(0);
        *to_balance += amount.amount;

        state.sequence += 1;
        let tx = TransactionRecord {
            id: Ulid::new().to_string(),
            created_at: Utc::now(),
            from_account_id: from_id.to_string(),
            to_account_id: to_id.to_string(),
            currency: amount.currency,
            amount: amount.amount,
            idempotency_key: idempotency_key.map(|s| s.to_string()),
            sequence: state.sequence,
        };

        if let Some(key) = &tx.idempotency_key {
            state.idempotency.insert(key.clone(), tx.clone());
        }

        state.transactions.push(tx.clone());
        Ok(public_transaction(&tx))
    }

    pub fn list_transactions(
        &self,
        limit: usize,
        after_sequence: u64,
    ) -> Result<(Vec<Transaction>, Option<u64>), LedgerError> {
        let mut limit = limit;
        if limit == 0 || limit > 1000 {
            limit = 100;
        }
        let state = self.inner.read().expect("ledger lock poisoned");
        let mut result = Vec::new();
        let mut last = None;
        for tx in state.transactions.iter() {
            if tx.sequence <= after_sequence {
                continue;
            }
            result.push(public_transaction(tx));
            last = Some(tx.sequence);
            if result.len() >= limit {
                break;
            }
        }
        Ok((result, last))
    }
}

fn to_public_account(id: impl Into<String>, record: &AccountRecord) -> Account {
    Account {
        id: id.into(),
        created_at: record.created_at,
        balances: record
            .balances
            .iter()
            .map(|(c, a)| (c.clone(), *a))
            .collect(),
    }
}

fn public_transaction(tx: &TransactionRecord) -> Transaction {
    Transaction {
        id: tx.id.clone(),
        created_at: tx.created_at,
        from_account_id: tx.from_account_id.clone(),
        to_account_id: tx.to_account_id.clone(),
        currency: tx.currency.clone(),
        amount: tx.amount,
        idempotency_key: tx.idempotency_key.clone(),
        sequence: tx.sequence,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;
    use std::thread;

    #[test]
    fn create_account_and_get_balance() {
        let ledger = Ledger::new();
        let account = ledger
            .create_account(Money::new("QZN", 1_000))
            .expect("create account");
        assert_eq!(account.id.len(), 26);
        let balance = ledger.get_balance(&account.id, "QZN").expect("balance");
        assert_eq!(balance.amount, 1_000);
    }

    #[test]
    fn transfer_success() {
        let ledger = Ledger::new();
        let a = ledger.create_account(Money::new("QZN", 1_000)).unwrap();
        let b = ledger.create_account(Money::new("QZN", 0)).unwrap();

        ledger
            .transfer(&a.id, &b.id, Money::new("QZN", 600), None)
            .expect("transfer");

        assert_eq!(ledger.get_balance(&a.id, "QZN").unwrap().amount, 400);
        assert_eq!(ledger.get_balance(&b.id, "QZN").unwrap().amount, 600);
    }

    #[test]
    fn transfer_insufficient() {
        let ledger = Ledger::new();
        let a = ledger.create_account(Money::new("QZN", 100)).unwrap();
        let b = ledger.create_account(Money::new("QZN", 0)).unwrap();

        let err = ledger
            .transfer(&a.id, &b.id, Money::new("QZN", 200), None)
            .unwrap_err();
        assert_eq!(err, LedgerError::InsufficientFunds);
    }

    #[test]
    fn transfer_idempotent() {
        let ledger = Ledger::new();
        let a = ledger.create_account(Money::new("QZN", 1_000)).unwrap();
        let b = ledger.create_account(Money::new("QZN", 0)).unwrap();

        let tx1 = ledger
            .transfer(&a.id, &b.id, Money::new("QZN", 200), Some("same"))
            .unwrap();
        let tx2 = ledger
            .transfer(&a.id, &b.id, Money::new("QZN", 200), Some("same"))
            .unwrap();

        assert_eq!(tx1.id, tx2.id);
        assert_eq!(ledger.get_balance(&a.id, "QZN").unwrap().amount, 800);
    }

    #[test]
    fn list_transactions_respects_limit() {
        let ledger = Ledger::new();
        let a = ledger.create_account(Money::new("QZN", 5000)).unwrap();
        let b = ledger.create_account(Money::new("QZN", 0)).unwrap();

        for _ in 0..5 {
            ledger
                .transfer(&a.id, &b.id, Money::new("QZN", 100), None)
                .unwrap();
        }

        let (txs, last) = ledger.list_transactions(3, 0).unwrap();
        assert_eq!(txs.len(), 3);
        assert!(last.is_some());
    }

    #[test]
    fn concurrent_transfers_preserve_total() {
        let ledger = Arc::new(Ledger::new());
        let a = ledger.create_account(Money::new("QZN", 10_000)).unwrap();
        let b = ledger.create_account(Money::new("QZN", 0)).unwrap();

        let mut handles = Vec::new();
        for _ in 0..10 {
            let ledger_clone = ledger.clone();
            let from = a.id.clone();
            let to = b.id.clone();
            handles.push(thread::spawn(move || {
                for _ in 0..10 {
                    let _ = ledger_clone.transfer(&from, &to, Money::new("QZN", 100), None);
                }
            }));
        }

        for handle in handles {
            handle.join().unwrap();
        }

        let total = ledger.get_balance(&a.id, "QZN").unwrap().amount
            + ledger.get_balance(&b.id, "QZN").unwrap().amount;
        assert_eq!(total, 10_000);
    }
}
