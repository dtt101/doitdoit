# AUR packaging

These source-repository files generate the separate `doitdoit-bin` AUR Git
repository. The AUR repository must contain only `PKGBUILD`, `.SRCINFO`,
`LICENSE` (0BSD), and any genuinely necessary helper files.

## Initial publication checklist

1. Recheck the official Arch repositories and AUR for both `doitdoit` and
   `doitdoit-bin` immediately before publishing.
2. Generate a dedicated revocable Ed25519 key with
   `ssh-keygen -t ed25519 -f ~/.ssh/aur-doitdoit`, add only its public key to
   the maintainer's AUR account, and configure it only for `aur.archlinux.org`.
3. Keep the GitHub repository variable `AUR_PUBLISH_ENABLED` unset. The first
   package must be reviewed and pushed manually.
4. Merge release-readiness changes, create the new `v0.2.0` tag, and let the
   release workflow publish new artifacts. Never reuse v0.1.8 artifacts.
5. Download the published checksums and run:

       ./packaging/aur/render.sh 0.2.0 doitdoit_0.2.0_checksums.txt /tmp/doitdoit-aur

6. In a clean Arch environment, run `makepkg --verifysource`, build in a clean
   chroot, inspect `pacman -Qlp`, perform install/remove smoke tests, and run
   `namcap` on both the PKGBUILD and package. Separately download and verify the
   aarch64 source checksum, metadata, and ELF architecture.
7. Clone `ssh://aur@aur.archlinux.org/doitdoit-bin.git`, copy only the reviewed
   generated files, commit to `master`, and push manually.

After the first manual publication, set a protected GitHub Environment named
`aur` with required reviewer approval, add the dedicated private key as the
`AUR_SSH_PRIVATE_KEY` environment secret, then set `AUR_PUBLISH_ENABLED=true`.
The automation still requires maintainer review; it serializes publication and
refuses equal-version or downgrade pushes.
