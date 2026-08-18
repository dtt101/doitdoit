# Publish doitdoit for Omarchy through the AUR

## Summary

Publish the prebuilt app as `doitdoit-bin` in the public AUR. Users install it with:

```bash
omarchy pkg aur add doitdoit-bin
```

The installed command remains `doitdoit`. Version v0.1.8 will be reviewed and published manually; later stable GitHub tags will update the AUR automatically.

## Application and package changes

- Add an explicit Omarchy first-run prompt before the TUI starts. It will show the exact destination and ask permission to install `~/.config/omarchy/hooks/theme-set.d/doitdoit`.
- Install the hook through the supported `omarchy hook install` command. The hook sends `SIGUSR2` to running `doitdoit` processes; declining makes no changes and prompts again on the next launch.
- Add `doitdoit config omarchy-hook install|status|remove`. Removal only deletes the managed hook and refuses to delete a user-modified file.
- Expand GoReleaser archives to include `LICENCE.md`.
- Add an AUR `PKGBUILD` template for x86_64 and ARM64. It will download the matching GitHub release archive, verify its SHA-256 checksum, install `/usr/bin/doitdoit`, README, and licence, and declare `provides/conflicts=('doitdoit')`.
- Document Omarchy installation, first-run theme integration, removal, and the maintainer release process.

## Initial publication

1. Create an AUR account and a dedicated, revocable Ed25519 key as recommended by the [AUR submission guide](https://wiki.archlinux.org/title/AUR_submission_guidelines).
2. Add the public key to the AUR account.
3. Merge the packaging changes while leaving `AUR_PUBLISH_ENABLED` unset.
4. Tag `v0.1.8`; the existing GoReleaser workflow creates the release archives and checksum file.
5. Render `PKGBUILD` and `.SRCINFO` for v0.1.8, build the x86_64 package locally without installing it, run package checks, and inspect its contents.
6. Clone the initially empty `ssh://aur@aur.archlinux.org/doitdoit-bin.git` repository, commit the reviewed metadata, and push the first version manually.
7. Add the dedicated private key and verified AUR host entry as GitHub secrets, add Git author details as repository variables, then set `AUR_PUBLISH_ENABLED=true`.

## Automated updates

- Add an AUR publication job after the existing GoReleaser job.
- Run it only for final tags matching `vMAJOR.MINOR.PATCH`; prereleases will not update the stable AUR package.
- Read the x86_64 and ARM64 hashes from GoReleaser's published checksum file, render `PKGBUILD`, generate `.SRCINFO` as a non-root user in an Arch container, and validate both sources.
- Clone the separate AUR repository over SSH, commit as `doitdoit-bin VERSION-1`, and push only when metadata changed.
- Use repository-owned shell steps rather than a third-party AUR publishing action.
- A failed AUR update will fail only the publication job; the GitHub release remains available and the job can be rerun.

## Test and acceptance plan

- Run all Go tests with a temporary `HOME`.
- Test Omarchy absent, hook accepted, hook declined, hook installation failure, existing current hook, outdated managed hook, and user-modified hook.
- Confirm tests never touch the developer's real `~/.config/omarchy`.
- Run `goreleaser check` and a snapshot release; inspect both Linux archives for the executable, README, and licence.
- Build and inspect the x86_64 Arch package, validate the ARM64 archive and checksum, and run `namcap`.
- Acceptance: a clean Omarchy user installs `doitdoit-bin`, runs `doitdoit`, approves the disclosed hook, switches themes, and sees every running instance recolour without restarting.

## Assumptions

- v0.1.8 is the first AUR release.
- Both x86_64 and ARM64 are supported.
- The package remains usable on plain Arch; Omarchy is an optional runtime integration.
- No implementation or test command will edit the current machine's live Omarchy, Hyprland, terminal, or theme configuration.
- AUR is the initial channel; proposing inclusion in Omarchy's curated repository can happen later after the package has users and release history.
