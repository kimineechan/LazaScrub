# LazaScrub usage guide

A portable Windows application that **lists the metadata in a file, writes a cleaned copy and checks that copy**. It reads photos (JPEG, PNG, GIF, WebP), PDFs and Word, Excel and PowerPoint files.

## Start here

1. Extract the ZIP to a folder on your PC.
2. Open `LazaScrub.exe`.
3. Choose one of the three sections, or drop a file or a folder anywhere on the window.

The app targets **64-bit Windows 10 and Windows 11**. You do not need Go, Node, Python or an installer. Nothing is uploaded: the app has no network client.

You can also drag a file onto `LazaScrub.exe` in Explorer, or pass a path on the command line, to open it straight away.

## 01 Inspect a file

- Select **Browse...** and choose a file, or drop one on the window.
- The table lists every piece of metadata found, the most revealing first.
- Select a row to read the complete value and a short explanation under the table.
- **Save report...** writes the list to a file. Choose the type in the Save dialog: a text report to read, or JSON for other tools. Both include the file's size and SHA-256 hash.
- **Clean a copy...** takes the file to the next section.

Inspecting only reads the file. Nothing is changed or written unless you save a report.

| Column | Meaning |
| --- | --- |
| Risk | High identifies a person, place or device. Medium describes how and when the file was made. Low is technical |
| Where | The part of the file that holds the value, such as EXIF, XMP or Document properties |
| Field | The name of the value |
| Value | What is stored |

For a picture inside a document, **Where** shows the path to it, for example `Picture word/media/image1.jpeg > GPS`.

## 02 Clean and verify

1. Select **Browse...** and choose a file, or arrive here from the first section.
2. Choose a name for the cleaned copy. The suggestion is the original name with `-clean` added.
3. LazaScrub writes the copy, scans it again and compares it with the original.

The two lines above the table give the outcome:

- How many fields were removed, and how many were kept on purpose.
- Whether the content of the copy matches the original. For a picture this compares the compressed picture data, so the two files decode to the same pixels. For a PDF it compares every object the document uses. For an Office file it compares every part, apart from the names and IDs that were removed.

The table lists each field as **Removed**, **Kept** or **Still present**:

- **Kept** fields stay because the file needs them or because they are content. Select the row to read the reason. Examples are a photo's orientation and colour profile, or text that Word still stores for a tracked deletion.
- **Still present** means a field should have been removed but was found again in the copy. This should not happen. If it does, do not rely on the copy, and please report the file type.

**Show in folder** opens the folder that holds the copy. **Save report...** writes the before-and-after result as text or JSON, with the SHA-256 hash of both files.

The original file is never changed. LazaScrub refuses to save the copy over it.

## 03 Clean a folder

1. Select **Browse...** and choose a folder, or drop a folder on the window.
2. The table lists each supported file with the number of fields it holds and how many are high risk.
3. Tick **Include subfolders** to go through the folders inside it as well.
4. Select **Clean all files**.

The cleaned copies go into a new folder beside the original, with `-clean` added to its name. An existing folder is never reused: if `Photos-clean` exists, the next run writes `Photos-clean-2`. Subfolders keep their layout.

Files of other types are not copied, so the clean folder holds only files that were cleaned. A file that could not be cleaned is left out, and the **Result** column says why.

## Command line

`lazascrub-cli.exe` runs the same engine from a terminal or a script.

```powershell
.\lazascrub-cli.exe scan photo.jpg
.\lazascrub-cli.exe scan --json report.pdf budget.xlsx
.\lazascrub-cli.exe clean photo.jpg
.\lazascrub-cli.exe clean report.pdf shared\report.pdf
.\lazascrub-cli.exe folder --subfolders "C:\Scans"
```

| Command | Result |
| --- | --- |
| `scan <file>...` | Prints a report for each file. `--json` prints JSON instead |
| `clean <file> [<copy>]` | Writes a cleaned copy and prints what was removed, kept and left. Without a second name the copy is `<name>-clean` beside the original. An existing file is not overwritten |
| `folder <folder>` | Cleans every supported file into a new `-clean` folder. `--subfolders` includes the folders inside it |
| `version` | Prints the version |

The exit code is 0 on success and 1 when a file could not be read or cleaned.

## What is kept on purpose

| Kept | Why |
| --- | --- |
| Orientation of a rotated photo | Without it the picture would be shown on its side |
| Colour profile and resolution | Removing them changes how the picture looks or prints |
| Tracked changes, comments, hidden sheets and slides | They are content. LazaScrub removes the names and dates attached to them, not the text |
| Attachments, macros, scripts, links, data connections | The document depends on them |

Everything that is kept appears in the verification table, so nothing stays in a file without being shown.

## Limits

| Limit | Detail |
| --- | --- |
| File types | JPEG, PNG, GIF, WebP, PDF, Word, Excel and PowerPoint. Other types are not read |
| Size | 1 GiB per file |
| Protected PDFs | Reported, not cleaned. Remove the password first |
| Signed PDFs | Cleaning invalidates the signature |
| Motion photos | The video stored after the picture is removed with the other trailing data |
| Storage | Local files only; the app has no network client or telemetry |

LazaScrub removes the metadata it recognises and shows what remains. It cannot judge what the visible content of a file gives away, and no tool can promise that a file is free of every identifying detail. Check the verification table before sharing a sensitive file.

For the full list of what is found in each file type, see [What LazaScrub finds and removes](FORMATS.md). For source builds and development, see [the README](../README.md).
