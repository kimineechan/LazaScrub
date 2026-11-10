# Contributing

Thanks for helping improve LazaScrub.

## Report a problem

Use the bug-report form and include the Windows version, the LazaScrub version, the file type and the steps to reproduce. Quote any error text.

The most useful report is a file that LazaScrub reads incorrectly. **Do not attach a file that holds real personal data.** Describe what made it, such as the camera model or the app and its version, or make a small test file with invented details.

For feature requests, describe the workflow and the result you want.

## Develop

Install Go 1.25 or newer. The repository has no third-party Go modules.

Tests on any platform:

```sh
go test ./internal/...
go run ./cmd/verify
```

Format Go changes with `gofmt`. On Windows, run `build.cmd` before submitting a change. It regenerates the resources, runs `go vet` and the tests, builds both programs and runs the round trip on sample files.

### Layout

| Path | Contents |
| --- | --- |
| `internal/meta` | The finding type and the risk rules shared by every reader |
| `internal/exif`, `xmp`, `iptc`, `icc` | Readers for metadata blocks that several file types embed |
| `internal/jpeg`, `png`, `gif`, `webp`, `pdf`, `office` | One package per file type, each with `Scan`, `Clean` and `Fingerprint` |
| `internal/scrub` | Chooses the handler, cleans to a new file and verifies the result |
| `internal/report` | Text and JSON reports |
| `internal/samples` | Builds sample files with invented metadata for the tests |
| `internal/winui`, `cmd/lazascrub` | The native Windows interface |
| `cmd/lazascrub-cli`, `cmd/verify` | The command-line tool and the release check |
| `tools/mkres` | Draws the icon and builds the Windows resource object |

### Rules for a file type handler

- `Clean` must never re-encode picture data or rewrite content it does not need to touch.
- Everything `Scan` reports must be either removed by `Clean` or marked as kept, with the reason in its note.
- `Fingerprint` must cover the content that `Clean` promises to preserve, and nothing that it removes.
- A damaged or hostile file must not crash a reader. Add the file type to the truncation tests.
- Add the sample to `internal/samples` and to `cmd/verify`, with invented names and places only.

To regenerate the icon and resources after changing the design, the manifest or `VERSION`:

```sh
go run ./tools/mkres
```

Commit the regenerated `assets/lazascrub.ico` and `cmd/lazascrub/resources_windows_amd64.syso`.

## Pull requests

Keep a pull request focused. Explain the problem, the resulting behaviour, and the checks performed. Add tests when a change affects reading, cleaning or verification.

## Releases

Update `VERSION`, `CHANGELOG.md`, and `docs/releases/v<version>.md` together, regenerate the resources, and push the matching `v<version>` tag. The release workflow builds on Windows, runs the tests, packages both programs, and publishes the ZIP and checksums after the Linux and Windows jobs pass.

See [the publishing guide](docs/GITHUB.md).
