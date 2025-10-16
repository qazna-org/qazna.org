-- Core ledger schema

create table if not exists accounts (
  id text primary key,
  created_at timestamptz not null default now()
);

create table if not exists balances (
  account_id text not null references accounts(id) on delete cascade,
  currency text not null,
  amount bigint not null default 0,
  primary key (account_id, currency)
);

create table if not exists transactions (
  id text primary key,
  created_at timestamptz not null default now(),
  from_account_id text not null references accounts(id),
  to_account_id   text not null references accounts(id),
  currency text not null,
  amount bigint not null,
  sequence bigserial not null unique,
  idempotency_key text unique
);

create index if not exists idx_transactions_sequence on transactions(sequence);
create index if not exists idx_transactions_from on transactions(from_account_id);
create index if not exists idx_transactions_to   on transactions(to_account_id);
