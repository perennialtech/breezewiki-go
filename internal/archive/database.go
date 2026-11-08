package archive

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Archive owns the same storage/archiver.db and storage/archive hierarchy
// used by the original BreezeWiki archiver.
type Archive struct {
	Storage string
	DB      *sql.DB
	Client  HTTPDoer
	Origin  func(wiki string) string
}

var migrations = [][]string{
	{
		`create table page (wikiname TEXT NOT NULL, basename TEXT NOT NULL, progress INTEGER NOT NULL, PRIMARY KEY (wikiname, basename))`,
		`create table wiki (wikiname TEXT NOT NULL, progress INTEGER, PRIMARY KEY (wikiname))`,
	},
	{`create table special_page (wikiname TEXT NOT NULL, key TEXT NOT NULL, basename TEXT NOT NULL, PRIMARY KEY (wikiname, key))`},
	{`update wiki set progress = 2 where wikiname in (select wikiname from wiki inner join page using (wikiname) group by wikiname having min(page.progress) = 1)`},
	{`create table image (wikiname TEXT NOT NULL, hash TEXT NTO NULL, url TEXT NOT NULL, ext TEXT, source INTEGER NOT NULL, progress INTEGER NOT NULL, PRIMARY KEY (wikiname, hash))`},
	{
		`alter table wiki add column sitename TEXT`,
		`alter table wiki add column basepage TEXT`,
		`alter table wiki add column license_text TEXT`,
		`alter table wiki add column license_url TEXT`,
	},
	{`alter table page add column redirect`},
}

func Open(storage string) (*Archive, error) {
	if err := os.MkdirAll(storage, 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(storage, "archiver.db"))
	if err != nil {
		return nil, err
	}
	// The original archiver enables WAL. Use a single connection so that all
	// operations and pragmas share it, including concurrent GUI activities.
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, err
	}
	if err = migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Archive{Storage: storage, DB: db}, nil
}

func (a *Archive) Close() error { return a.DB.Close() }

func migrate(db *sql.DB) error {
	_, err := db.Exec(`create table if not exists database_version (version integer, primary key (version))`)
	if err != nil {
		return err
	}
	var version int
	err = db.QueryRow(`select version from database_version`).Scan(&version)
	if err == sql.ErrNoRows {
		if _, err = db.Exec(`insert into database_version values (0)`); err != nil {
			return err
		}
		version = 0
	} else if err != nil {
		return err
	}
	if version < 0 || version > len(migrations) {
		return fmt.Errorf("unsupported archive database version %d", version)
	}
	for version < len(migrations) {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		for _, statement := range migrations[version] {
			if _, err = tx.Exec(statement); err != nil {
				break
			}
		}
		if err == nil {
			_, err = tx.Exec(`update database_version set version = ?`, version+1)
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("archive migration %d: %w", version+1, err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("archive migration %d: %w", version+1, err)
		}
		version++
	}
	return nil
}
