CREATE INDEX oauth_pending_expiry ON oauth_pending(expires);
CREATE INDEX oauth_clients_expiry ON oauth_clients(expires);
CREATE INDEX oauth_credentials_grant_expiry ON oauth_credentials(grant_id,kind,expires);
