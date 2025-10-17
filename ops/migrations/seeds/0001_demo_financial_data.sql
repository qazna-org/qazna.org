-- Seed data for demonstration environments.

insert into organizations (id, name, metadata)
values
  ('org-central-kaz', 'National Bank of Qazakhstan', jsonb_build_object('region', 'Eurasia')),
  ('org-central-sng', 'Union Reserve Cooperative', jsonb_build_object('region', 'CIS')),
  ('org-monetary-eu', 'European Monetary Authority', jsonb_build_object('region', 'EU'))
on conflict (id) do nothing;

insert into users (id, organization_id, email, password_hash)
values
  ('usr-admin-kaz', 'org-central-kaz', 'chief@nbqz.gov', '$argon2id$v=19$m=65536,t=2,p=1$demo$hash'),
  ('usr-ops-eu', 'org-monetary-eu', 'ops@ema.eu', '$argon2id$v=19$m=65536,t=2,p=1$demo$hash')
on conflict (id) do nothing;

insert into roles (id, organization_id, name, description)
values
  ('role-sysadmin', 'org-central-kaz', 'system_admin', 'Full platform administration'),
  ('role-supervisor', 'org-monetary-eu', 'supervisor', 'Comprehensive oversight role')
on conflict (id) do nothing;

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

insert into role_permissions (role_id, permission_id) values
  ('role-sysadmin', 'perm-ledger-transfer'),
  ('role-sysadmin', 'perm-ledger-account'),
  ('role-sysadmin', 'perm-observe'),
  ('role-sysadmin', 'perm-auth-org'),
  ('role-sysadmin', 'perm-auth-users'),
  ('role-sysadmin', 'perm-auth-roles'),
  ('role-sysadmin', 'perm-auth-perms'),
  ('role-supervisor', 'perm-observe')
on conflict do nothing;

insert into user_roles (user_id, role_id, organization_id) values
  ('usr-admin-kaz', 'role-sysadmin', 'org-central-kaz'),
  ('usr-ops-eu', 'role-supervisor', 'org-monetary-eu')
on conflict do nothing;

insert into accounts (id) values
  ('acct-sovereign-001'),
  ('acct-sovereign-002'),
  ('acct-sovereign-003')
on conflict do nothing;

insert into balances(account_id, currency, amount) values
  ('acct-sovereign-001', 'QZN', 10000000000),
  ('acct-sovereign-002', 'QZN', 7500000000),
  ('acct-sovereign-003', 'USD', 2000000000)
on conflict (account_id, currency) do update set amount = excluded.amount;

insert into transactions(id, from_account_id, to_account_id, currency, amount, idempotency_key)
values
  ('txn-demo-0001', 'acct-sovereign-001', 'acct-sovereign-002', 'QZN', 500000000, 'demo-0001'),
  ('txn-demo-0002', 'acct-sovereign-002', 'acct-sovereign-003', 'USD', 250000000, 'demo-0002')
on conflict (id) do nothing;
