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
defaults is available from the binary's `-help-config` option.

The default port is `10416`. A `config.ini` file is optional. The server embeds
its CSS, JavaScript, fonts, and artwork; it does not need the source checkout or
a `static/` directory after installation. To use a custom `static/` directory,
pass `-root /path/to/root` (where that root contains `static/`). Use
`-config /path/to/config.ini` for a different configuration file.

Example `config.ini`:

```ini
application_name = BreezeWiki
bind_host = 127.0.0.1
port = 10416
canonical_origin = http://localhost:10416

[feature_jsonp]
enabled = true

[access_log]
enabled = true
```

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
docker run --rm -p 10416:10416 ghcr.io/perennialtech/breezewiki-go:latest
```

Open `http://localhost:10416`. Alternatively, use the included `compose.yaml`:

```sh
docker compose up -d
```

The container listens on all interfaces at port `10416`, uses embedded assets,
and supports the same `BW_` environment variables as the native server. For
example, pass `-e BW_ACCESS_LOG::ENABLED=true` to `docker run` to log requests.

## Other tools and development

Run `go run ./cmd/breezewiki-wikinames` to update `wiki-names.json` from
Fandom's public wiki directory. The file is generated and ignored by Git.

Run `make ci` for `go test ./...`, `go vet ./...`, and `go build ./...`;
`go test -race ./...` also passes. Integration tests use a local mock
MediaWiki API to verify pages, search, and static resources.

This is a language rewrite, not a byte-identical Racket port. Go's HTML5
parser can differ on malformed markup, and Racket's development hot-reload
workflow is replaced by building and rerunning the Go binary.
