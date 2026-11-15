# Changelog

## 1.1.0 - 2026-11-15

A new window, in the style of LazaReplay. What LazaScrub finds and removes is unchanged.

- The window is black and see-through, with what is behind it blurred, and has rounded buttons, a switch for **Include subfolders** and its own tables.
- The tables scroll with the mouse wheel, the bar at their right and the arrow keys, Page Up, Page Down, Home and End.
- Keyboard use throughout: Tab and Shift + Tab move between controls with a white ring to show where you are, Enter or Space presses, and Ctrl + Tab changes section.
- The button for the thing to do next is the light one.
- The dialog for saving a cleaned copy opens in the folder of the original.
- The window is drawn by LazaScrub itself, only where something changed. No web view, no third-party modules, CGO still off.

## 1.0.0 - 2026-11-10

First public release.

- Inspect JPEG, PNG, GIF and WebP pictures, PDFs, and Word, Excel and PowerPoint files, and rate every field as high, medium or low risk.
- Read EXIF, GPS, XMP, IPTC, Photoshop resources, colour profiles, comments and data hidden after the end of a picture.
- Read PDF document information, XMP, review comments, thumbnails and the objects that earlier saves left behind, including replaced property values.
- Read Office properties, comment and tracked-change authors, accounts, session IDs, template paths, saved-in folders and printer settings.
- Find metadata in pictures and documents embedded in PDFs and Office files.
- Report the Windows download record and other hidden streams beside a file.
- Write a cleaned copy without touching the original, then scan it again and compare a fingerprint of its content with the original.
- Clean every supported file in a folder into a new folder.
- Save reports as text or JSON, with SHA-256 hashes.
- Include a command-line tool, Windows build and release automation, and tests.
