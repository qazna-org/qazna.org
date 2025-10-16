-- Drop oauth client seed

delete from oauth_clients where id = 'demo-client';

drop table if exists oauth_clients;
