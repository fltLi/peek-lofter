# Data formats

## Ref

A Ref is produced by `tag`, `user` and `merge`. It is the input format for
`post --from` and `browse`.

~~~json
{
  "id": 123456789,
  "url": "https://example-user.lofter.com/post/abc_def",
  "permalink": "abc_def",
  "title": "A short title",
  "author": {
    "blogId": 1000001,
    "blogName": "example-user",
    "nickname": "nickname"
  },
  "publishedAt": "2024-05-05T12:36:12.387+08:00",
  "type": 2,
  "source": {
    "kind": "tag",
    "name": "example-tag",
    "window": "example-tag:20241101T0000-20241130T2359"
  }
}
~~~

| Field | Required | Notes |
| --- | --- | --- |
| `id` | yes | LOFTER post id |
| `url` | yes | Canonical public URL |
| `permalink` | no | `blogbase_postbase` |
| `title` | no | May be empty |
| `author` | yes | Blog identity |
| `publishedAt` | yes | RFC3339Nano in UTC+8 |
| `type` | no | 1 text, 2 image, 4 video |
| `source` | no | Where the ref was discovered |

`merge` dedupes by id, then normalized URL, then `blogName` + permalink. It keeps
the first record and fills empty fields from later duplicates.

## Work archive

`post` writes one zip per work:

~~~text
<out>/<blogId>/<postId>.zip
  meta.json
  content.html
  images/001.jpg    # only with --with-media
~~~

`meta.json` is a Post record without `content`; `content.html` holds the original
HTML. Images are stored only with `--with-media`. `post` does not generate txt,
md or a manifest.

`meta.json` embeds Ref and may add:

~~~json
{
  "tags": ["example-tag", "example-fandom"],
  "stats": {"hot": 371, "favorite": 249, "comments": 18},
  "images": [
    {"raw": "https://imglf6.lf127.net/img/...", "orign": "https://..."}
  ],
  "flags": {
    "payView": false,
    "fansVipPrivate": false,
    "forbidVideoDownload": false
  }
}
~~~

Archives are written through a staging directory and a temporary zip, then
renamed into place. Existing valid archives are skipped unless `--force` is set.
`post` scans `<out>` and reads `meta.json` to match by id, URL or permalink before
making any request.

## Export output

`export` reads a work archive and writes derived files:

~~~text
txt     plain text
md      Markdown
html    original content.html
~~~

`--format txt,md` writes both. `--with-title` prepends the title to txt/md only.
Layouts:

~~~text
id              <blogId>/<postId>.<ext>
author-id       <author>/<postId>.<ext>
author-title    <author>/<title>.<ext>
title           <title>.<ext>
~~~

`--author-field blogName|nickname` selects the author value. Empty titles become
`untitled`; empty authors become `unknown-author`. Illegal Windows characters and
trailing dots/spaces are removed. Conflicts use `<name>_<postId>.<ext>` by
default; identical content is skipped, and `--force` overwrites.

## Tag dataset

`peek-lofter tag --out DIR` creates:

~~~text
DIR/
  refs.jsonl
  windows.jsonl
  manifest.json
  state.json        last sync timestamp
~~~

### refs.jsonl

One Ref per line. The file is append-only. Readers dedupe by `id`; `browse` and
`merge` use id-first dedupe.

### windows.jsonl

One WindowEvent per line. Events may be appended by multiple runs; the latest
event for a window describes its current outcome. Windows are calendar months.

~~~json
{
  "window": "example-tag:20241101T0000-20241130T2359",
  "tag": "example-tag",
  "kind": "month",
  "status": "complete",
  "key": "1,2,4,",
  "pages": 17,
  "items": 151,
  "inside": 151,
  "outside": 0,
  "startedAt": "2026-11-15T01:46:43+08:00",
  "endedAt": "2026-11-15T01:46:49+08:00"
}
~~~

Statuses:

| Status | Meaning |
| --- | --- |
| `complete` | Paged to the end with no out-of-window items |
| `partial` | Paged to the end but some boundary items were outside the window |
| `empty` | No posts in the month |
| `suspect` | The month kept returning out-of-window data |
| `unresolved` | Hit the monthly cap; the API cannot split it further |

Only `complete` windows are considered done for resume. A crawl with `suspect` or
`unresolved` windows exits 5.

### manifest.json

~~~json
{
  "version": 1,
  "tags": ["example-tag"],
  "since": "2024-01-01T00:00:00+08:00",
  "until": "2024-12-31T00:00:00+08:00",
  "types": "1,2,4",
  "createdAt": "2026-11-15T01:46:43+08:00",
  "lastRunAt": "2026-11-15T01:46:49+08:00",
  "works": 177
}
~~~

`works` counts refs appended in the current dataset; it is not a deduplicated
total.

### state.json

Resume state. It stores the last sync time, is written atomically, and can be
removed safely; it is also recovered from `windows.jsonl`.

## Reading refs

`post --from` accepts:

- Ref JSONL produced by `tag`, `user` or `merge`.
- A plain text file with one URL or permalink per line.
- CSV with a `url` header; the first column is used.

`browse` accepts Ref JSONL, work zips, directories containing either, and `-` for
Ref JSONL on stdin. Blank lines are ignored. Duplicate ids or URLs are processed
once.
