create table if not exists oauth_auth_codes (
  code text primary key,
  client_id text not null references oauth_clients(id) on delete cascade,
  code_challenge text not null,
  code_challenge_method text not null,
  redirect_uri text not null,
  user_id text not null,
  roles jsonb not null,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  consumed_at timestamptz
);

create index if not exists idx_oauth_auth_codes_client on oauth_auth_codes(client_id);
