# Experimental web companion follow-up

The Dropbox web companion remains separate from the CLI release gate. The
v0.3 stabilization pass completed the original engineering checklist:

- Removed remote scripts and fonts; the companion now has no browser package
  inventory to update.
- Added a restrictive CSP with only the required Dropbox connection sources.
- Documented JavaScript-accessible token storage and implemented best-effort
  Dropbox token revocation on disconnect.
- Published a plain-language privacy and recovery disclosure.
- Made forever retention the default and positive pruning explicitly opt-in.
- Added domain and revision-conflict tests plus downloadable recovery copies.

It is still experimental: a serverless Dropbox client cannot protect its
refresh token from a compromised browser context, and conflict recovery is
manual rather than a semantic merge. Do not describe it as covered by the CLI
release's security review or data-handling guarantees until those constraints
receive a separate review.
