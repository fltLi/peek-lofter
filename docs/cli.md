# CLI reference

## Commands

| Command | Input | Output | Pager |
| --- | --- | --- | --- |
| `tag` | network | Ref JSONL | no |
| `user` | network | Ref JSONL | no |
| `post` | Ref | work zip | no |
| `browse` | Ref JSONL / work zip | list, Ref, or zip paths | yes |
| `export` | work zip | txt, md, or html | no |
| `merge` | Ref JSONL or work zip | merged Ref or work library | no |

Data is written to stdout. Progress, warnings, summaries and pager prompts are
written to stderr. `--quiet` prints errors only and never suppresses data.
`--quiet` and `--verbose` are mutually exclusive.

## Global flags

| Flag | Default | Description |
| --- | --- | --- |
| `--format` | auto | `auto`, `list`, `table`, `jsonl`, `json`, `ids` or `urls` |
| `--fields` | all | Comma-separated field projection for JSON/JSONL |
| `--rate` | 350ms | Minimum interval between requests |
| `--timeout` | 30s | Per-request timeout |
| `--retries` | 3 | Retries for transient errors |
| `--concurrency` | 2 | Download/export concurrency where applicable |
| `--quiet` | false | Print errors only |
| `--verbose` | false | Print per-period and per-post details |
| `--version` | — | Print the version |
| `--help` | — | Print help |

With `--format auto`, a TTY gets `list` and a pipe gets JSONL. `list` includes
the post id. `table` is aligned and flushed at the end.

## tag

~~~text
peek-lofter tag <tag...> [flags]
~~~

Crawls a tag by calendar month, newest first. A bare invocation walks back from
the current month and stops after six consecutive empty months.

| Flag | Default | Description |
| --- | --- | --- |
| `--since` | auto | Start date (`YYYY-MM-DD` or RFC3339) |
| `--until` | now | End date; date-only values are inclusive |
| `--out` | — | Write a dataset; omit to stream Ref JSONL |
| `--limit` | 0 | Stop after N refs; 0 means all |
| `--empty-months` | 6 | Stop after N consecutive empty months; 0 means never |
| `--type` | all | `all`, `text`, `image` or `video` |
| `--strict` | false | Exit 5 on an incomplete period |
| `--resume` | true | Skip completed periods when `--out` is set |

With `--since`, the full range is scanned unless `--empty-months` is explicitly
set. `--out` writes `refs.jsonl`, `windows.jsonl`, `manifest.json` and
`state.json`.

~~~bash
peek-lofter tag example-tag --limit 10
peek-lofter tag example-tag --since 2024-01-01 --out ./data/example-tag
peek-lofter tag example-tag --empty-months 12 --out ./data/example-tag
~~~

## user

~~~text
peek-lofter user <user>
peek-lofter user posts <user> [flags]
peek-lofter user likes <user> [flags]
peek-lofter user recommends <user> [flags]
~~~

`user <user>` prints the profile and monthly archive counts. List subcommands
emit Ref JSONL.

| Flag | Default | Description |
| --- | --- | --- |
| `--limit` | 0 | Stop after N refs; 0 means all |
| `--out` | — | Write a Ref dataset instead of stdout |

`user posts` also accepts `--since` and `--until`.

~~~bash
peek-lofter user example-user
peek-lofter user posts example-user --limit 20
peek-lofter user likes example-user --limit 10 --out ./refs/likes
~~~

## post

~~~text
peek-lofter post <ref...> [flags]
peek-lofter post --from <file> [flags]
~~~

Downloads each post into one zip archive. `post` never writes txt, md or html;
use `export` for those formats.

| Flag | Default | Description |
| --- | --- | --- |
| `--from` | — | Read refs from a file; repeatable, `-` means stdin |
| `--blog` | — | Blog name for numeric ids |
| `--out` | `peek-out` | Archive root |
| `--with-media` | false | Store images in the archive |
| `--with-video` | false | Keep video metadata; video files are not downloaded |
| `--force` | false | Overwrite existing archives |
| `--json` | false | Emit one result object per post |
| `--dry-run` | false | Print planned refs without fetching |
| `--limit` | 0 | Maximum refs to process; 0 means all |

Before fetching, `post` scans `<out>` for zips, reads `meta.json`, and matches
by id, URL or permalink. A complete match is reported as `skipped` without a
network request. `--force` bypasses the scan. Archives are:

~~~text
<out>/<blogId>/<postId>.zip
  meta.json
  content.html
  images/001.jpg    # only with --with-media
~~~

~~~text
post
  total     57
  saved     50
  skipped    6
  failed     1
  elapsed   2m10s
  output    dl

failures
  https://example-user.lofter.com/post/abc_def  network timeout
~~~

`--json` emits `status`, `id`, `url`, `title`, `path`, and `error` when present.

~~~bash
peek-lofter post https://example-user.lofter.com/post/abc_def --out ./dl --with-media
peek-lofter post --from refs.jsonl --out ./dl
peek-lofter post --from refs.jsonl --out ./dl --force
peek-lofter post --from refs.jsonl --dry-run --format jsonl
~~~

## browse

~~~text
peek-lofter browse [path...] [flags]
~~~

Local-only browse and search. Inputs may be Ref JSONL, work zips, directories
containing either, or `-` for Ref JSONL on stdin. A directory with both refs and
works is merged by id; work records win. The default path is `peek-out`.

| Flag | Default | Description |
| --- | --- | --- |
| `--query`, `-q` | — | Case-insensitive substring over title, author, tags and URL |
| `--author` | — | Author blogName or nickname |
| `--tag` | — | Include tag; repeatable, any match |
| `--exclude-tag` | — | Exclude tag; repeatable, any match |
| `--type` | all | `all`, `text`, `image` or `video` |
| `--since` | — | On/after this date |
| `--until` | — | Before this date |
| `--content` | false | Search downloaded HTML, works only |
| `--page` | 1 | Show one page |
| `--page-size` | 20 | Rows per page |
| `--no-pager` | false | Print all rows |
| `--paths` | false | Print work zip paths only |

Tags are case-insensitive; a leading `#` is ignored. Exclusions win over
inclusions. `--paths` is the bridge to `export`; `--format jsonl` is the bridge
to `post`.

~~~text
page 1/3  items 1-20 of 57
ID        DATE        AUTHOR        TITLE                                     URL
123456    2024-05-05  example-user  Example title                             https://example-user.lofter.com/post/abc_def

[n]ext  [p]rev  [q]uit:
~~~

~~~bash
peek-lofter browse ./data/refs.jsonl
peek-lofter browse ./dl -q "keyword" --exclude-tag nsfw
peek-lofter browse ./dl --page 2 --page-size 20
peek-lofter browse ./dl --paths
~~~

## export

~~~text
peek-lofter export <zip|dir...> [flags]
~~~

Reads work zips and writes derived files. It never performs network requests.
Inputs are zip files, directories containing zips, or zip paths on stdin (`-`).
Passing `refs.jsonl` is an error; run `post` first.

| Flag | Default | Description |
| --- | --- | --- |
| `--format` | txt | `txt`, `md`, `html`; comma-separated for multiple |
| `--out` | `export` | Output root |
| `--layout` | author-title | `id`, `author-id`, `author-title` or `title` |
| `--author-field` | blogName | `blogName` or `nickname` |
| `--with-title` | false | Include title in txt/md content |
| `--on-conflict` | suffix | `suffix`, `skip` or `error` |
| `--force` | false | Overwrite existing exports |
| `--stdout` | false | Write one archive and one format to stdout |
| `--dry-run` | false | Print planned paths without writing |

~~~text
id              <blogId>/<postId>.<ext>
author-id       <author>/<postId>.<ext>
author-title    <author>/<title>.<ext>
title           <title>.<ext>
~~~

Empty titles become `untitled`; empty authors become `unknown-author`. Windows
illegal characters and trailing dots/spaces are removed. A conflict uses
`<name>_<postId>.<ext>` by default; identical content is skipped.

~~~bash
peek-lofter export ./dl --format txt --out ./text
peek-lofter export ./dl --format txt,md --with-title --out ./text
peek-lofter export ./dl --format txt --layout id --out ./text
peek-lofter export ./dl/1/123456.zip --format txt --stdout
peek-lofter browse ./dl -q "keyword" --paths | peek-lofter export - --format txt --out ./text
~~~

## merge

~~~text
peek-lofter merge <input...> [flags]
~~~

Auto-detects the input kind. All inputs must be Ref JSONL or all must be work
zips/directories; mixed input is an error.

Ref mode:

~~~bash
peek-lofter merge a.jsonl b.jsonl -o refs.jsonl
~~~

Refs are deduped by id, then normalized URL, then `blogName` + permalink.
Duplicates keep the first record and fill empty fields from later records.
Omitting `--out` writes to stdout.

Work mode:

~~~bash
peek-lofter merge ./dl-a ./dl-b --out ./dl
~~~

Work archives are deduped by post id and copied to
`<out>/<blogId>/<postId>.zip`. The first archive wins; `--force` lets later
archives overwrite. Existing destination files are skipped unless `--force` is
set. No network requests are made.

| Flag | Default | Description |
| --- | --- | --- |
| `--out`, `-o` | — | Ref output file, or work output directory |
| `--force` | false | Overwrite existing work archives |

## version and completion

~~~bash
peek-lofter version
peek-lofter completion bash
peek-lofter completion zsh
peek-lofter completion fish
peek-lofter completion powershell
~~~

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success, including an empty result |
| 1 | Unexpected error |
| 2 | Usage or flag error |
| 3 | Target not found |
| 4 | Network or upstream failure |
| 5 | Partial success: some periods, posts or exports failed |

Errors are written to stderr as `error: ...`. Data is never written to stderr.
