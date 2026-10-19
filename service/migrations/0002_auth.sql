-- OAuth state shares D1's atomicity; credentials are stored only as SHA-256 digests.
CREATE TABLE oauth_clients(id TEXT PRIMARY KEY, metadata TEXT NOT NULL, expires INTEGER NOT NULL);
CREATE TABLE oauth_pending(digest TEXT PRIMARY KEY, kind TEXT NOT NULL, browser TEXT NOT NULL, payload TEXT NOT NULL, expires INTEGER NOT NULL);
CREATE TABLE oauth_grants(id TEXT PRIMARY KEY, owner TEXT NOT NULL, client TEXT NOT NULL, scope TEXT NOT NULL, resource TEXT NOT NULL, revoked INTEGER NOT NULL DEFAULT 0);
CREATE TABLE oauth_credentials(digest TEXT PRIMARY KEY, kind TEXT NOT NULL, grant_id TEXT NOT NULL REFERENCES oauth_grants(id), expires INTEGER NOT NULL, redirect TEXT, challenge TEXT);
CREATE INDEX oauth_credentials_grant ON oauth_credentials(grant_id);
-- Exchange a code/refresh token and mint its successors as one SQLite statement.
CREATE TABLE oauth_exchanges(id TEXT PRIMARY KEY, predecessor TEXT NOT NULL, kind TEXT NOT NULL, client TEXT NOT NULL, resource TEXT NOT NULL, now INTEGER NOT NULL, redirect TEXT NOT NULL, challenge TEXT NOT NULL, access_digest TEXT NOT NULL, refresh_digest TEXT);
CREATE TRIGGER oauth_exchange BEFORE INSERT ON oauth_exchanges BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM oauth_credentials c JOIN oauth_grants g ON c.grant_id=g.id
    WHERE c.digest=NEW.predecessor AND c.kind=NEW.kind AND c.expires>NEW.now
      AND g.client=NEW.client AND g.resource=NEW.resource AND g.revoked=0
      AND (c.kind='refresh' OR (c.redirect=NEW.redirect AND c.challenge=NEW.challenge))
  ) THEN RAISE(ABORT,'invalid_grant') END;
END;
CREATE TRIGGER oauth_exchange_apply AFTER INSERT ON oauth_exchanges BEGIN
  INSERT INTO oauth_credentials(digest,kind,grant_id,expires)
    SELECT NEW.access_digest,'access',grant_id,NEW.now+900 FROM oauth_credentials WHERE digest=NEW.predecessor;
  INSERT INTO oauth_credentials(digest,kind,grant_id,expires)
    SELECT NEW.refresh_digest,'refresh',grant_id,NEW.now+2592000 FROM oauth_credentials WHERE digest=NEW.predecessor AND NEW.refresh_digest IS NOT NULL;
  -- Keep consumed credentials for replay detection until expiry. They cannot be exchanged twice.
  UPDATE oauth_credentials SET kind='used_' || kind WHERE digest=NEW.predecessor;
  DELETE FROM oauth_exchanges WHERE id=NEW.id;
END;
