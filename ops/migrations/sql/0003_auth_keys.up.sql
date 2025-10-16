create table if not exists auth_keys (
  id bigserial primary key,
  kid text not null unique,
  public_pem text not null,
  private_pem text not null,
  created_at timestamptz not null,
  rotated_at timestamptz,
  expires_at timestamptz not null,
  status text not null check (status in ('active','retired'))
);

create index if not exists idx_auth_keys_status on auth_keys(status);
