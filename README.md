# BreezeWiki Go

Go implementation of [BreezeWiki](https://gitdab.com/cadence/breezewiki),
a privacy-focused alternative frontend for Fandom wikis.

Licensed under AGPL-3.0, as is the upstream project. This repository
retains original assets and credits the upstream BreezeWiki contributors.

## Offline archiving

Run the downloader with a Fandom wiki subdomain:

```sh
go run ./cmd/breezewiki-archiver minecraft
```

The default `storage/archiver.db` stores progress across interrupted runs.
Page JSON, CSS and downloaded style images are kept under
`storage/archive/<wikiname>/` in the original BreezeWiki archive layout.
Run the same command again to resume. Use `-q`, `-l`, or `-p` to select
quiet, filename, or terminal progress output.

A desktop control panel supports queued downloads, pause/resume, reset,
and reset-page-list actions:

```sh
go run ./cmd/breezewiki-archiver-gui
```

It opens a local browser window. Downloads and their progress remain in
SQLite when the application closes; restart it to resume incomplete wikis.

To index downloaded pages in Solr, create a core named for the wiki using
the supplied `archiver/solr-config-dir` configuration, then run:

```sh
go run ./cmd/breezewiki-index minecraft
```

The indexer supports `.json` and `.json.gz` pages, writes the original
`cache.rkt` document cache, and offers `-c` to reuse it. Each command's
`--help` lists its options, including alternate storage and Solr paths.
