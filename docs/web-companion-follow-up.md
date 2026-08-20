# Experimental web companion follow-up

The Dropbox web companion is separate from the v0.2.0 CLI/AUR release gate.
Before it can share that release's security or privacy claims, complete and
review all of the following:

- Pin, inventory, and monitor every remotely hosted or self-hosted browser
  dependency; document the update and vulnerability-response process.
- Define and deploy a restrictive Content Security Policy, including explicit
  Dropbox API/connect sources and no unnecessary script execution paths.
- Threat-model OAuth token storage, reduce token lifetime and scope where
  possible, protect tokens from script access, and document sign-out/revocation.
- Publish a plain-language privacy disclosure covering Dropbox processing,
  browser-local data, hosting logs, and the absence or presence of telemetry.
- Implement retention settings and migration semantics that are compatible
  with the CLI, including forever mode and a guarantee of no pruning before an
  explicit saved choice.
- Add backup/recovery guidance and concurrent-edit tests for Dropbox sync.

Until those items are complete, do not describe the web companion as covered
by the AUR package's security review or data-handling guarantees.
