# AGENTS.md

Maintainer notes for peek-lofter. User-facing usage lives in README.md; CLI
details live in docs/cli.md and data contracts in docs/datasets.md.

## Scope

peek-lofter is a read-only LOFTER CLI with six commands:

- `tag` crawls public tag feeds by calendar month.
- `user` reads public profiles, posts, likes and recommendations.
- `post` downloads Ref records into one zip per work.
- `browse` browses and searches local Ref JSONL and work zips.
- `export` derives txt, md or html from work zips.
- `merge` merges Ref JSONL or downloaded work libraries.

It does not log in, use cookies, access private or paid posts, provide a TUI,
daemon or database, or guarantee a complete archive.

## Repository layout

~~~text
cmd/peek-lofter      entrypoint
internal/cli         Cobra commands, flags, local pager, exit codes
internal/config      global defaults and env overrides
internal/dwr         DWR JavaScript reply parser
internal/library     Ref/zip scanning, filters and sorting
internal/render      list / table / JSONL / JSON / ids / urls writers
pkg/model            Ref, Post, User, Timestamp, query and page types
pkg/lofter           HTTP client for api.lofter.com and www.lofter.com
pkg/tag              planner, month windows, cold keys, runner
pkg/user             profile, posts, likes, recommends
pkg/post             resolver, detail fetch, zip archive, export, Markdown
pkg/store            refs.jsonl / windows.jsonl / state.json / manifest.json
pkg/media            image URL migration and download
pkg/version          build metadata injected with ldflags
~~~

Dependency direction:

~~~text
cmd -> internal/cli -> pkg/tag, pkg/user, pkg/post, internal/library, internal/render, internal/config
pkg/tag  -> pkg/lofter, pkg/store, pkg/model
pkg/user -> pkg/lofter, pkg/model
pkg/post -> pkg/lofter, pkg/media, pkg/model
internal/library -> pkg/post, pkg/model
pkg/lofter -> pkg/model, internal/dwr
pkg/store  -> pkg/model
internal/render -> pkg/model
~~~

Public packages must not import Cobra, Viper, progressbar or go-pretty.
CLI-only dependencies stay in internal/.

## Build and checks

~~~bash
go build ./...
go test ./...
go vet ./...
golangci-lint run ./...
~~~

CI uses Go from go.mod, runs tests on Linux, and builds on Linux, Windows and
macOS. Releases are built with GoReleaser; ldflags set pkg/version fields.

## Data contracts

`model.Ref` is the exchange format between discovery, download and browse:

~~~json
{
  "id": 123,
  "url": "https://example-user.lofter.com/post/abc_def",
  "permalink": "abc_def",
  "title": "title",
  "author": {"blogId": 1, "blogName": "example-user", "nickname": "name"},
  "publishedAt": "2024-05-05T12:36:12+08:00",
  "type": 1,
  "source": {"kind": "tag", "name": "example-tag", "window": "..."}
}
~~~

`model.Post` embeds Ref and adds tags, stats, content, images and flags.
Timestamps serialize as RFC3339Nano in UTC+8 via a fixed zone; Windows builds do
not depend on tzdata.

## Tag crawling

1. `tag.Planner` emits calendar-month windows, newest first. The first window
   may be a partial month up to `--until`.
2. All types are `postTypes=1,2,4`; `1,2,3,4,5,6` collapses to one result.
3. Each request sends `postYm=YYYYMM`. `postYmdSt/Et` are ignored, so date
   splitting below a month is not usable.
4. Each month gets a distinct cold key by appending empty type tokens.
5. `tag.Runner` pages until `next == -1` or the ~1010 cursor cap. Out-of-window
   refs are counted but not emitted.
6. Too many out-of-window refs triggers a fresh cold-key retry. A cap hit is
   `unresolved`; it cannot be split further.
7. Refs are deduped by post id across windows and tags.
8. `--out` appends refs and WindowEvents to a dataset. Only `complete` windows
   are skipped on resume.
9. `--limit` is global. `--empty-months` stops walk-back after N empty months;
   0 disables it. An explicit `--since` scans the full range unless
   `--empty-months` is passed explicitly.

Window statuses are `complete`, `partial`, `empty`, `suspect` and `unresolved`.
Suspect or unresolved windows make the command exit 5. There is no complete
archive guarantee.

## Work archives

`pkg/post.Fetcher` always requests full detail and writes:

~~~text
<out>/<blogId>/<postId>.zip
  meta.json
  content.html
  images/001.jpg    # only with --with-media
~~~

`meta.json` is a Post with `content` removed. There is no manifest. Archives are
built through a staging directory and a temporary zip, then renamed into place.

Before fetching, `FetchAll` scans `<out>` for zip files, reads `meta.json` and
indexes by post id, normalized URL and permalink. A complete hit is emitted as
`skipped` without any request. `--force` bypasses the scan. Corrupt or incomplete
archives are refetched. There is no catalog or database.

`ExportOne` reads `meta.json` and `content.html` and writes txt/md/html. Layouts:
`id`, `author-id`, `author-title` (default) and `title`. `--author-field` selects
`blogName` or `nickname`. Conflicts compare bytes, then suffix, skip or error.
Exports never make network requests.

## Browse and pager

`browse` scans paths with `internal/library`. Inputs may be Ref JSONL, zips,
directories with `refs.jsonl`, directories with work zips, or `-` for Ref JSONL.
Duplicate ids prefer work records over refs. The default path is `peek-out`.

Filters cover query, author, tag include/exclude, type and date range. Tags are
case-insensitive, strip a leading `#`, and exclusions win. `--content` reads
HTML from work zips only.

Pagination exists only in `browse`. It is line-based (`n`, `p`, `q`), not raw
mode or a full-screen TUI. `--page` prints one page, `--page-size` controls page
size, `--no-pager` prints everything, and non-TTY output defaults to JSONL.
`--paths` emits zip paths for export.

## Merge

`merge` auto-detects all-Ref or all-work inputs and rejects mixed input. Ref mode
dedupes by id/URL/permalink, keeps the first record and fills empty fields. Work
mode copies zips to `<out>/<blogId>/<postId>.zip`, dedupes by post id, and keeps
the first archive; `--force` lets later archives overwrite. No network.

## Upstream API notes

- Tag feed: `POST api.lofter.com/newapi/tagPosts.json`, 10 items/page, cursor
  in `data.offset`, cap around 1010 per query.
- User posts: `POST api.lofter.com/v2.0/blogHomePage.api`, page size up to 50.
- H5 detail: `GET www.lofter.com/newweb/post/detail.json?permalink=...` is cheap
  but has no `photoLinks`.
- Full detail: `POST api.lofter.com/oldapi/post/detail.api` has content,
  photoLinks, embed and permission flags. It needs numeric post and blog ids.
- Likes/recommends use anonymous DWR `BlogBean.queryLikePosts` and
  `BlogBean.querySharePosts`. They return posts, not users.
- DWR replies are JavaScript assignments. `internal/dwr` scans assignments, never
  evaluates JS, and takes the post id from the object.
- Images: rewrite `nos.netease.com/imglfN/...` to `imglfN.lf127.net/...`; prefer
  raw, fall back to orign.

## CLI output

- All CLI-facing strings are English. README is Chinese; data is not translated.
- stdout carries data, including Ref JSONL, JSON results and `--stdout` exports.
- stderr carries progress, warnings, summaries and pager prompts.
- `post`, `export` and `merge` print summaries and never prompt. `browse` is the
  only pager.
- `--quiet` prints errors only and never suppresses data. `--quiet` and
  `--verbose` are mutually exclusive.
- `list` is the streaming TTY format. `table` is aligned and flushed at the end.
  Pipes default to JSONL.

## Testing

- HTTP mapping and tag crawling use httptest fixtures.
- Tag planning and window logic are pure unit tests.
- DWR parsing uses fixture strings.
- Archive paths, zip write/read, export layouts and text conversion are unit
  tested.
- Library scanning, paging, Ref merging and work merging use t.TempDir.
- CLI help, local browse, export, merge and post skip behavior use injected
  buffers, temp archives and httptest.
- Live smoke tests are optional and rate limited.

## Caveats

- Tag crawls can be partial; never describe a dataset as a complete archive.
- `--with-video` keeps metadata only; video files are not downloaded.
- Private, paid and follower-only content is skipped.
- Default rate is 350ms with concurrency 2; keep rate limits for bulk crawling.
- DWR endpoints are disallowed by robots.txt; the user bears the risk.
- Go 1.26; release builds disable CGO.

## Documentation

- README.md is the only Chinese document and stays user-facing.
- Other documentation and code comments are English.
- Package docs live in doc.go files.
- Current behavior belongs in README usage, docs/cli.md and docs/datasets.md; this
  file records architecture and implementation details.
