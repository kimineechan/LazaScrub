# Publish LazaScrub on GitHub

## Repository settings

| Setting | Value |
| --- | --- |
| Owner | `kimineechan` |
| Name | `LazaScrub` |
| Visibility | Public |
| Default branch | `main` |
| Licence | MIT |
| First release tag | `v1.0.0` |
| First release title | `LazaScrub v1.0.0` |

**Description**

```text
Portable Windows utility that lists the metadata hidden in photos, PDFs and Office documents, removes it, and verifies the cleaned copy.
```

**Topics**

```text
windows, golang, desktop-app, metadata, exif, privacy, pdf, forensics, metadata-removal, portable-app, win32, windows-utility
```

These settings are also stored in `.github/repository.json` for reference. GitHub does not apply that file automatically.

## How a release is made

A release is built and published by GitHub Actions when a version tag is pushed. You do not need Go installed to publish one.

1. Change `VERSION`, add an entry to `CHANGELOG.md`, and create `docs/releases/v<version>.md`.
2. If you have Go installed, run `go run ./tools/mkres` so the version shown in the EXE's properties matches, and commit the changed resource file. The Windows build checks this and fails if it was forgotten.
3. Commit and push `main`.
4. Create and push the matching version tag:

```powershell
git tag -a v1.0.1 -m "LazaScrub v1.0.1"
git push origin v1.0.1
```

The tag must match `VERSION`. Each tagged release uses the source at that tag. Use a new version instead of moving an existing release tag.

The **Build and release** workflow then:

1. Runs the tests on Linux with the race detector, and the round trip on sample files.
2. Builds both programs on Windows, runs the tests there, and packages the ZIP with its SHA-256 checksums.
3. Publishes the ZIP and `SHA256SUMS.txt` as a release, using `docs/releases/v<version>.md` as the notes, after both jobs pass.

Follow the run under the repository's **Actions** tab. The release appears under **Releases** when it finishes.

If the workflow fails, open the failing job in Actions, fix the error, and push again. If the tag itself points at a broken commit, fix the problem and release the next patch version. A rerun after an already-published release does not overwrite it.

## Publish without the tag

To push changes without making a release, commit and push `main` only. The workflow still builds and tests every push, and keeps the built ZIP as a workflow artifact for 14 days.
