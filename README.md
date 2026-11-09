# breezewiki-go

A Go rewrite of [BreezeWiki](https://gitdab.com/cadence/breezewiki), an
alternative frontend for Fandom wikis. It serves Fandom wiki pages and search
results, removes unwanted Fandom elements, and provides local and proxied
static resources. The web server, HTML transformation, and wiki-directory
downloader are written in Go.

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

The complete list of supported configuration keys, descriptions, and
defaults is available from the binary's `-help-config` option. The generated
[`config.example.ini`](config.example.ini) and [`.env.example`](.env.example)
include every setting with its default and description. Run
`make generate-config-examples` after changing a setting; `make ci` checks
that both files are current.

The default port is `10416`. A `config.ini` file is optional. The server embeds
its CSS, JavaScript, fonts, and artwork; it does not need the source checkout or
a `static/` directory after installation. To use a custom `static/` directory,
pass `-root /path/to/root` (where that root contains `static/`). Use
`-config /path/to/config.ini` for a different configuration file.

Copy `config.example.ini` to `config.ini` to customise file-based settings.

`BW_` environment variables override file settings. Nested settings use `::`,
for example `env 'BW_FEATURE_JSONP::ENABLED=false' ./breezewiki` to load pages
server-side rather than through browser JSONP and a same-origin POST. Other
settings include `strict_proxy` and `feature_search_suggestions`. Theme and
notice preferences are stored in browser cookies.

The Fandom HTTP transport uses browser-compatible TLS and HTTP/2 settings
because Fandom's edge currently rejects Go's default HTTP fingerprint. Access
may still depend on Fandom's availability and anti-bot policies.

## Docker

Run the published image:

```sh
docker run --rm --read-only --cap-drop=ALL \
  --security-opt=no-new-privileges --publish 10416:10416 \
  ghcr.io/perennialtech/breezewiki-go:latest
```

Open `http://localhost:10416`. Alternatively, use the included `compose.yaml`:

```sh
docker compose up -d
```

The container runs as a non-root user, listens on all interfaces at port
`10416`, and uses embedded assets. Both examples run with a read-only root
filesystem, dropped Linux capabilities, and no privilege escalation. The
container supports the same `BW_` environment variables as the native server.
Copy `.env.example` to `.env` and pass `--env-file .env` to `docker run` to
customise them. Nested names use `::`, which shells cannot source as variables.
Alternatively, pass `-e BW_ACCESS_LOG::ENABLED=true` to `docker run` to log
requests.

## Compared with original BreezeWiki

This comparison uses the [original Racket implementation at `6d09507`](https://gitdab.com/cadence/breezewiki/src/commit/6d09507b6e7ecec0cbac4c2c4eb223c10ccc23a4).
Both versions serve Fandom articles, categories, file pages, search, themes,
and proxied resources. Both support server-side loading and browser JSONP;
those are not new features of the Go rewrite.

| Area | breezewiki-go | Original BreezeWiki |
| --- | --- | --- |
| Deployment | Compiled Go executable with embedded static assets; Docker/Compose and published `linux/amd64` and `linux/arm64` images | Racket runtime and packages; static assets served from the source tree |
| Discussions | Read-only public article comments, including replies and pagination | No article comments viewer |
| Navigation | Language-prefixed wiki routes, localized namespace aliases, `Special:Random` and paginated `Special:AllPages`; sortable wiki tables | Unprefixed wiki routes; no dedicated implementations of those special pages or bundled table sorting |
| Search and files | Requests up to 50 Fandom search results; displays file upload dates and links to additional file-usage results | Uses the API's default search result limit; further file uses are indicated with an ellipsis |
| Page layout | Bounded article width, horizontally scrollable wide tables, and compact independent-wiki notices | No article width cap; larger independent-wiki notices |
| Not ported | No offline archiving, Solr indexing/search, or built-in CAPTCHA | These optional subsystems are present (CAPTCHA disabled by default) |

### Local HTTP measurements

On 9 November 2026, both versions were run in containers from the same base
image on one Linux x86-64 host, using host networking. The Go executable was
built with Go 1.27.3 (`CGO_ENABLED=0`); the original used Racket 8.18 and
`dist.rkt`. The results below are medians of three warmed ApacheBench runs
with 800 requests and concurrency 8 for each route.

| Measurement | breezewiki-go | Original BreezeWiki |
| --- | ---: | ---: |
| Homepage (`GET /`), requests/second | 12,570 | 1,296 |
| Stylesheet (`GET /static/main.css`), requests/second | 18,108 | 5,402 |
| Server process RSS after the runs | 26.5 MiB | 230.3 MiB |

For each route, the command was `ab -q -n 800 -c 8 http://127.0.0.1:PORT/`
(substituting the server's port and `/static/main.css` for the stylesheet).
These endpoints do not fetch Fandom content. This is a local request-handling
comparison, **not** a benchmark of live wiki page loading, where upstream
latency, rate limits, and blocking can dominate. Memory is one post-test
process snapshot, not peak or total container usage.

## Other tools and development

Run `go run ./cmd/breezewiki-wikinames` to update `wiki-names.json` from
Fandom's public wiki directory. The file is generated and ignored by Git.

Run `make ci` for `go test ./...`, `go vet ./...`, and `go build ./...`;
`go test -race ./...` also passes. Integration tests use a local mock
MediaWiki API to verify pages, search, and static resources.

This is a language rewrite, not a byte-identical Racket port. Go's HTML5
parser can differ on malformed markup, and Racket's development hot-reload
workflow is replaced by building and rerunning the Go binary.
