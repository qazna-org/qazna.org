drop index if exists idx_audit_resource;
drop table if exists audit_log;

drop index if exists idx_user_roles_org;
drop table if exists user_roles;

drop table if exists role_permissions;
drop table if exists permissions;
drop index if exists idx_roles_org;
drop table if exists roles;

drop index if exists idx_refresh_tokens_user;
drop table if exists refresh_tokens;

drop index if exists idx_users_org;
drop table if exists users;

drop table if exists organizations;
