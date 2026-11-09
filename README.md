# breezewiki-go

A Go rewrite of [BreezeWiki](https://gitdab.com/cadence/breezewiki), an
alternative frontend for Fandom wikis. It serves the same page routes, removes
unwanted Fandom elements, offers local and proxied resources, and can read wikis
archived for offline use. The web server, HTML transformation, archive downloader,
indexer, archive control UI, and wiki-directory downloader are written in Go.

This project derives from BreezeWiki's source and keeps its **AGPL-3.0** license,
static assets, and attribution to the original BreezeWiki contributors. It is
not affiliated with Fandom.

## Build and run

Go 1.26 or later is required. Run `make ci` to test, vet, and build every
package. Build the server and run it with default configuration:

```sh
go build -o breezewiki ./cmd/breezewiki
./breezewiki
```

The default port is `10416`. A `config.ini` file is optional and follows the
original BreezeWiki setting names. The server embeds its CSS, JavaScript, fonts,
and artwork; it does not need the source checkout or a `static/` directory after
installation. To use a custom `static/` directory or a specific archive storage
location, pass `-root /path/to/root` (where that root contains `static/` and/or
`storage/`). Use `-config /path/to/config.ini` for a different configuration file.

Example `config.ini`:

```ini
application_name = BreezeWiki
bind_host = 127.0.0.1
port = 10416
canonical_origin = http://localhost:10416

[feature_jsonp]
enabled = true

[feature_offline]
enabled = false

[access_log]
enabled = true
```

`BW_` environment variables override file settings. Nested settings use `::`,
for example `env 'BW_FEATURE_JSONP::ENABLED=false' ./breezewiki` to load pages
server-side rather than through browser JSONP and a same-origin POST. The
original config option names, including `strict_proxy`, theme cookies,
`feature_search_suggestions`, and CAPTCHA settings, are retained.

The Fandom HTTP transport uses browser-compatible TLS and HTTP/2 settings
because Fandom's edge currently rejects Go's default HTTP fingerprint. Access
may still depend on Fandom's availability and anti-bot policies.

## Offline archiving

Download a wiki by its Fandom subdomain, without the `.fandom.com` suffix:

```sh
go run ./cmd/breezewiki-archiver examplewiki
```

The original `storage/archiver.db` schema is used. It keeps download progress
across interrupted runs and supports all six historical schema migrations.
Pages are saved in `storage/archive/<wikiname>/` along with styles and images.
Re-run the same command to resume. Output flags `-q`, `-l`, and `-p` select
quiet, per-file, and in-place progress modes.

The archiver also has an interactive download queue:

```sh
go run ./cmd/breezewiki-archiver-gui
```

This replacement for the original Racket desktop GUI opens a **local browser
window**, not a GTK or Qt application. It can add, pause, resume, reset, and
reset-page-list downloads, and shows progress and errors. Use `-no-browser`
to run the local control panel without opening a window.

Once a wiki has been downloaded, serve it from the same `storage/` directory.
For the archives produced by the Go archiver, use `format = json`:

```ini
[feature_offline]
enabled = true
only = true
format = json
search = fandom
```

The offline reader also accepts existing `.json.gz` archives when configured
with `format = json.gz`. In offline-only mode the web server doesn't fetch
missing wiki articles from Fandom; it shows the unavailable-page response.

## Solr indexing and search

For full-text search over offline archives, create an Apache Solr core named
for the wiki using `archiver/solr-config-dir`, then run:

```sh
go run ./cmd/breezewiki-index examplewiki
```

The indexer supports `.json` and `.json.gz` pages, document extraction,
batching, and the historical `cache.rkt` format; use `-c` to reuse a cache.
The indexer accepts `-solr-url`, `-storage`, and `-cache`. Set
`feature_offline::search = solr` in `config.ini` to search the local Solr core
from the web frontend (default Solr endpoint: `http://localhost:8983`).

## Other tools and development

Run `go run ./cmd/breezewiki-wikinames` to update `wiki-names.json` from
Fandom's public wiki directory. The file is generated and ignored by Git.

Run `make ci` for `go test ./...`, `go vet ./...`, and `go build ./...`;
`go test -race ./...` also passes. The integration tests use a local mock
MediaWiki API and SQLite archives. The server and the four-stage archiver were
also exercised against live Fandom pages; an archived wiki was then served
through the offline reader.

This is a language rewrite, not a byte-identical Racket port. Go's HTML5
parser can differ on malformed markup; the archive control UI runs in a local
browser window; and Racket's development hot-reload workflow is replaced by
building and rerunning the Go binary. As with upstream, archived CSS does not
recursively download `@import` resources or mirror inline article images.
