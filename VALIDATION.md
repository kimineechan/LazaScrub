# Validation

## Checks completed on 10 November 2026

Both programs were compiled with Go 1.27.3 for `windows/amd64`, with CGO disabled, and run on Windows 10.

### Automated tests

- Every reader has tests for what it finds, what cleaning removes, and what cleaning keeps. They use sample files built in code with invented names, places and serial numbers, so no real person's files are in the repository.
- After cleaning, each test searches the cleaned bytes for the sample's author, camera serial number, place names, comments and hidden data. None may survive.
- Cleaned JPEG, PNG and GIF samples are decoded and compared with the decoded original, pixel for pixel. Cleaning a clean file must change nothing.
- Cleaned Office samples are unpacked and checked: every XML part must still parse, every relationship must point at a part that exists, and every content type override must name a part that exists.
- The PDF samples cover a classic cross-reference table, a compressed object stream located through a cross-reference stream with a row predictor, a document saved twice, an unusable table, an incomplete table and a password-protected file.
- Every reader is run against truncated and corrupted copies of its sample. None may crash.
- `cmd/verify` writes one sample of each of the nine supported kinds to disk, cleans it through the same code the app uses, and fails if a field is left, if the content fingerprint changes or if the original is touched.

### Real files

Sample files prove that a reader matches this project's own understanding of a format. Real files prove more, so the command-line tool was also run over files that were already on the development PC. Only totals were recorded.

| Set | Files | Result |
| --- | --- | --- |
| Pictures and a manual shipped with Windows and installed apps | 328 (243 PNG, 64 JPEG, 20 GIF, 1 PDF) | All cleaned, with nothing left and the content fingerprint unchanged |
| Personal documents and pictures | 773 (400 PNG, 240 JPEG, 77 PDF, 21 GIF, 16 WebP, 10 Word, 9 PowerPoint) | All cleaned, with nothing left and the content fingerprint unchanged |

The largest file was a 247 MB PDF, which took under two seconds.

A fingerprint computed by the same code that did the cleaning cannot catch a mistake in how that code reads a file. So documents were also rendered by an independent program: LibreOffice 26.2 drew the first page of the original and of the cleaned copy, and the two pictures were compared byte for byte.

| Type | Files | Identical render |
| --- | --- | --- |
| PDF | 45 | 45 |
| Word | 10 | 10 |
| PowerPoint | 9 | 9 |

Word, Excel and PowerPoint files written by LibreOffice itself were cleaned as well, then opened and exported by LibreOffice without error.

### A fault this found

The render comparison caught a fault that every automated check had missed. Cross-reference streams without an `Index` entry were read as empty, which produced incomplete copies of four PDFs saved for fast web view. The project's own compressed sample had passed only because a fallback path masked the fault.

The reader was corrected and three safeguards were added:

- The tests fail if a sample is read through the fallback path instead of its own cross-reference data.
- `Clean` refuses to write a PDF in which it can find no pages, or whose table leaves out an object that the document refers to and that is present in the file.
- A cleaned PDF is read back before it is saved. It must open with its own table, have the same number of pages and lead to the same number of objects.

After the fix all 45 PDFs rendered identically, and all 77 real PDFs were read through their own cross-reference data.

### Interface

The three sections of the app were exercised on Windows 10 at 100% display scaling, with sample files: inspecting, cleaning through the Save dialog, verifying, and cleaning a folder. The screenshots in `docs/screenshots` come from those runs.

## GitHub checks

The included workflow runs the tests on Linux with the race detector, builds both programs on Windows, runs the tests there, checks that the committed icon and resources match what `tools/mkres` generates, and packages a ZIP with SHA-256 checksums. A version tag publishes the release only after both jobs succeed.

## Where the testing stops

- Cleaned Office files were opened with LibreOffice only. Microsoft Office was not available on the test machine.
- No workbook written by Microsoft Excel was available. Excel handling was checked with generated workbooks and with workbooks written by LibreOffice.
- The refusal of password-protected PDFs was checked with a generated file only.
- The interface was not run on Windows 11 or at display scaling other than 100%. It uses the same layout approach as LazaBinder, which scales its fixed layout to the display.
- The render comparison covers the first page of each document.
- Real files vary without limit. A file from an app that was not represented here may hold metadata in a place LazaScrub does not look. The verification screen shows what is left in a cleaned copy for that reason.
