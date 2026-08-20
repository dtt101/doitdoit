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

## Publication status

- [x] Implement and locally verify the release-readiness changes.
- [x] Confirm the README describes retention, overwrite-safe moves, the
  opt-in Omarchy hook, removal, backup guidance, privacy, and AUR installation.
- [x] Run tests, race tests, vet, vulnerability scanning, licence inventory
  validation, GoReleaser validation, and a snapshot release.
- [x] Inspect both snapshot Linux archives and confirm that the x86-64 and
  aarch64 executables are statically linked and have the expected architecture.
- [x] Recheck the official Arch repositories and AUR for package-name conflicts.
- [x] Commit and push the release-readiness changes to `main`.
- [ ] Wait for GitHub CI on the final release commit to pass.
- [ ] Create and push the annotated `v0.2.0` tag from that exact commit.

## Remaining initial-publication steps

1. Wait for the tag-triggered GitHub release workflow to finish, then apply
   `docs/releases/v0.2.0.md` as the public GitHub release notes if necessary.
2. Download every published v0.2.0 artifact and verify the GoReleaser checksum
   file with `sha256sum -c`. Inspect both Linux archives again for `doitdoit`,
   `README.md`, `LICENSE`, and `THIRD_PARTY_NOTICES.md`; confirm static linkage
   and the x86-64/aarch64 ELF architectures.
3. Create a dedicated revocable Ed25519 AUR publishing key and register only
   its public key with the maintainer's AUR account. Verify the AUR SSH host
   fingerprint independently. Keep `AUR_PUBLISH_ENABLED` unset.
4. Render `PKGBUILD`, `.SRCINFO`, and the 0BSD packaging licence from the
   published checksums with `packaging/aur/render.sh`.
5. Run `makepkg --verifysource`, `namcap`, package-content inspection, and an
   x86-64 clean-chroot build. Verify the aarch64 source URL, checksum, metadata,
   and extracted binary architecture separately.
6. On a disposable Arch environment, install the built package, smoke-test
   `doitdoit`, remove the package, and confirm package removal preserves task
   data and user configuration.
7. Clone `ssh://aur@aur.archlinux.org/doitdoit-bin.git` and manually push the
   first `master` commit. Commit only `PKGBUILD`, `.SRCINFO`, and the packaging
   `LICENSE`; do not copy source-repository or release-build files into the AUR
   repository.
8. On a disposable clean Omarchy installation, install with
   `omarchy pkg aur add doitdoit-bin`, confirm the safe retention default, then
   install the hook explicitly. Start multiple instances, change the theme,
   and confirm every instance retints. Remove the hook and package and verify
   that user data remains intact.
9. After the manual first publication is accepted, create the protected GitHub
   `aur` Environment with required reviewer approval, add the dedicated key as
   `AUR_SSH_PRIVATE_KEY`, and set `AUR_PUBLISH_ENABLED=true` for later stable
   releases only.

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
