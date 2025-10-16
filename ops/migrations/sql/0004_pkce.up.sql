-- OAuth client + PKCE seed for demonstrations

create table if not exists oauth_clients (
  id text primary key,
  secret text not null,
  name text not null,
  redirect_uri text not null,
  created_at timestamptz not null default now()
);

insert into oauth_clients(id, secret, name, redirect_uri)
values
  ('demo-client', 'demo-secret', 'Synthetic Sovereign Console', 'http://localhost:8080/callback')
on conflict (id) do update set secret = excluded.secret, name = excluded.name, redirect_uri = excluded.redirect_uri;
