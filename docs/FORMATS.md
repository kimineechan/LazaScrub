# What LazaScrub finds and removes

Every finding has a risk rating:

| Rating | Meaning |
| --- | --- |
| High | Identifies a person, a place or one specific device, or exposes content that was meant to be gone |
| Medium | Describes how and when the file was made: software, dates, document IDs, free text |
| Low | Technical values such as exposure settings or page counts |

"Kept" means the cleaned copy still has it, on purpose. The verification screen lists every kept item with the reason.

## JPEG

| Found | Cleaned copy |
| --- | --- |
| EXIF: camera make and model, lens, exposure, dates, time zone, software, artist, owner, serial numbers, unique image ID, maker note | Removed |
| GPS: position, altitude, speed, direction, satellite time | Removed |
| Embedded preview image (EXIF thumbnail, JFIF thumbnail, Photoshop thumbnail) | Removed |
| XMP, including extended XMP: creator, editing history, document IDs, face regions, keywords | Removed |
| IPTC caption data and Photoshop resources | Removed |
| Comments and other application segments, such as content credentials | Removed |
| Multi-picture index and anything stored after the end of the picture | Removed |
| Orientation | Kept when the picture is rotated, so it stays the right way up |
| Colour profile, resolution and the Adobe colour marker | Kept, because they affect how the picture is displayed |

The compressed picture data is copied byte for byte. It is never decoded or re-encoded.

## PNG

| Found | Cleaned copy |
| --- | --- |
| Text chunks, plain, compressed and international: author, software, comments, creation time | Removed |
| The prompt and settings that AI image generators store as text | Removed |
| XMP, the EXIF chunk and the hex-encoded profiles written by ImageMagick | Removed |
| Last-modified time | Removed |
| Private chunks added by editors | Removed |
| Anything stored after the end of the picture | Removed |
| Gamma, colour profile, resolution, transparency, palette and animation chunks | Kept |

## GIF

| Found | Cleaned copy |
| --- | --- |
| Comment blocks | Removed |
| XMP | Removed |
| Application blocks other than animation looping and the colour profile | Removed |
| Anything stored after the end of the file | Removed |
| Frames, timing, looping and the colour profile | Kept |

## WebP

| Found | Cleaned copy |
| --- | --- |
| EXIF and XMP chunks | Removed, and the header flags are updated to match |
| Unknown chunks | Removed |
| Anything stored after the end of the file | Removed |
| Picture data, transparency, animation and the colour profile | Kept |
| Orientation | Kept when the picture is rotated |

## PDF

| Found | Cleaned copy |
| --- | --- |
| Document information: title, author, subject, keywords, creating app, producer, dates, custom properties | Removed |
| XMP on the document and on individual pages and pictures | Removed |
| Document ID | Removed |
| Earlier versions: objects that later saves replaced or deleted, including old property values | Removed. Only the objects the document still uses are written |
| Page thumbnails | Removed |
| Private editing data stored by the creating app | Removed |
| Names and dates on review comments and highlights | Removed. The text of the comment stays |
| Metadata inside pictures stored as JPEG, such as GPS and serial numbers | Removed |
| Anything stored after the end of the file | Removed |
| Attachments, JavaScript | Reported and kept. They are part of the document |
| Digital signatures | Reported. Cleaning invalidates them |
| Password protection | Reported. The file is not cleaned |

A cleaned PDF is checked before it is saved: it must open with its own cross-reference table, have the same number of pages and lead to the same number of objects. If the original's table and its contents disagree, LazaScrub refuses to clean it instead of guessing.

## Word, Excel and PowerPoint

| Found | Cleaned copy |
| --- | --- |
| Properties: author, last modified by, company, manager, dates, revision number, editing time, template, application, statistics | Removed |
| Custom properties, often added by a document management system | Removed |
| Thumbnail of the first page | Removed |
| Names on comments and tracked changes, and the dates of each change | Names become "Author" and dates are removed, as Office does when asked to remove personal information |
| Accounts of the people who commented, which can include an email address | Removed |
| Editing session IDs, which link documents that share an origin | Removed |
| Path of the attached template | Removed |
| Folder the workbook was saved in | Removed |
| Printer settings, including the names of network printers | Removed |
| Save times recorded for each part of the package | Set to the fixed date that Office writes |
| Metadata in embedded pictures, PDFs and Office documents | Removed |
| Text deleted with change tracking on | Reported and kept. Accept or reject the changes in Word to remove it |
| Hidden sheets and hidden slides | Reported and kept |
| Macros, custom XML parts, embedded objects from other apps | Reported and kept |
| Links to local files, linked workbooks, data connections | Reported and kept, because the document depends on them |

## Windows

| Found | Cleaned copy |
| --- | --- |
| Download record: the zone a file came from, the address it was downloaded from and the page that linked to it | The copy is a new file, so it has no record |
| Other hidden data streams beside the file | The copy has none |

These live in the file system, not in the file. They follow a file to other NTFS drives but not through email or uploads.
