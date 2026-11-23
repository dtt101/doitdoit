-- Index access-token expiry for bounded cleanup and drop redundant grant index.
DROP INDEX IF EXISTS oauth_credentials_grant;
CREATE INDEX oauth_credentials_kind_expiry ON oauth_credentials(kind, expires);
