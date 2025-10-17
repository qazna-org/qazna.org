-- RBAC and audit schema

create table if not exists organizations (
  id text primary key,
  name text not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  metadata jsonb not null default '{}'::jsonb
);

create table if not exists users (
  id text primary key,
  organization_id text not null references organizations(id) on delete cascade,
  email text not null unique,
  password_hash text not null,
  status text not null default 'active',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists refresh_tokens (
  id text primary key,
  user_id text not null references users(id) on delete cascade,
  token_hash text not null,
  expires_at timestamptz not null,
  created_at timestamptz not null default now(),
  revoked boolean not null default false
);

create index if not exists idx_refresh_tokens_user on refresh_tokens(user_id);

create table if not exists roles (
  id text primary key,
  organization_id text not null references organizations(id) on delete cascade,
  name text not null,
  description text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (organization_id, name)
);

create table if not exists permissions (
  id text primary key,
  key text not null unique,
  description text,
  created_at timestamptz not null default now()
);

create table if not exists role_permissions (
  role_id text not null references roles(id) on delete cascade,
  permission_id text not null references permissions(id) on delete cascade,
  primary key (role_id, permission_id)
);

create table if not exists user_roles (
  user_id text not null references users(id) on delete cascade,
  role_id text not null references roles(id) on delete cascade,
  organization_id text not null references organizations(id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key (user_id, role_id)
);

create table if not exists audit_log (
  id text primary key,
  occurred_at timestamptz not null default now(),
  actor_user_id text,
  actor_org_id text,
  action text not null,
  resource_type text not null,
  resource_id text not null,
  metadata jsonb not null default '{}'::jsonb,
  trace_id text
);

create index if not exists idx_users_org on users(organization_id);
create index if not exists idx_roles_org on roles(organization_id);
create index if not exists idx_user_roles_org on user_roles(organization_id);
create index if not exists idx_audit_resource on audit_log(resource_type, resource_id);

insert into permissions (id, key, description)
values
  ('perm-ledger-transfer', 'ledger.transfer', 'Authorize ledger transfers'),
  ('perm-ledger-account', 'ledger.account.create', 'Authorize account creation'),
  ('perm-observe', 'platform.observe', 'View audit and observability data'),
  ('perm-auth-org', 'auth.manage_organizations', 'Manage organizations'),
  ('perm-auth-users', 'auth.manage_users', 'Manage organization users'),
  ('perm-auth-roles', 'auth.manage_roles', 'Manage organization roles'),
  ('perm-auth-perms', 'auth.manage_permissions', 'Manage role permissions')
on conflict (id) do nothing;
