# Publish doitdoit v0.2.0 through the AUR

The first AUR release is the newly built v0.2.0 `doitdoit-bin` package. It must
not reuse the v0.1.8 tag, archives, or checksums. Users install it with:

```bash
omarchy pkg aur add doitdoit-bin
```

The installed command remains `doitdoit`; Omarchy is an optional integration
and plain Arch is supported.

## Release gates

- MIT `LICENSE` and complete `THIRD_PARTY_NOTICES.md` ship in every archive.
- Retention is explicitly chosen and persisted before data loading; forever is
  the safe default. Storage moves never overwrite an existing destination.
- The Omarchy hook has no launch-time prompt or automatic mutation. Users opt
  in with `doitdoit config omarchy-hook install`; status and exact managed-file
  removal are also available.
- The experimental Dropbox companion remains outside this AUR release's
  security/privacy assurance until its tracked follow-up is complete.
- Tests, race tests, vet, vulnerability scanning, licence inventory validation,
  and GoReleaser validation pass with an isolated home directory.

## Initial publication

1. Recheck official Arch package names and the AUR immediately before release.
2. Create and register a dedicated revocable Ed25519 AUR key.
3. Leave `AUR_PUBLISH_ENABLED` unset.
4. Merge the release-readiness changes and create the new v0.2.0 tag.
5. Inspect the published archives and hashes, including static linkage and the
   aarch64 ELF architecture.
6. Render the AUR files, run `makepkg --verifysource`, build in a clean x86_64
   chroot, inspect contents, smoke-test install/removal, and run `namcap`.
7. Push the first separate AUR `master` commit manually with only `PKGBUILD`,
   `.SRCINFO`, its 0BSD packaging licence, and necessary helpers.

## Later automated updates

Only stable `vMAJOR.MINOR.PATCH` releases generate updates. Publication stays
disabled unless `AUR_PUBLISH_ENABLED=true`, is serialized, and runs through a
protected `aur` GitHub Environment with required reviewer approval. It uses a
dedicated SSH key, verifies the AUR host's published Ed25519 fingerprint,
refuses equal versions and downgrades, and uses minimal workflow permissions.
Automation does not replace maintainer review.

No implementation or test command may edit the developer's live Omarchy,
Hyprland, terminal, or theme configuration. Final end-to-end acceptance runs
only on a disposable clean Omarchy installation.
