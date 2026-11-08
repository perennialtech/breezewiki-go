package archive

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestDatabaseSchemaAndReopen(t *testing.T) {
	storage := t.TempDir()
	a, err := Open(storage)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err := a.DB.QueryRow(`select version from database_version`).Scan(&version); err != nil || version != 6 {
		t.Fatalf("version: %d, %v", version, err)
	}
	var mode string
	if err := a.DB.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal mode: %q, %v", mode, err)
	}
	if _, err := a.DB.Exec(`insert into wiki(wikiname,progress,sitename) values ('demo', 2, 'Demo')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(`insert into page(wikiname,basename,progress,redirect) values('demo','Hello',1,NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(`insert into special_page(wikiname,key,basename) values('demo','long','Hello')`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.DB.Exec(`insert into image(wikiname,hash,url,source,progress) values('demo','abc','https://example.net/x',1,0)`); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a, err = Open(storage)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var name string
	if err := a.DB.QueryRow(`select sitename from wiki where wikiname='demo'`).Scan(&name); err != nil || name != "Demo" {
		t.Fatalf("reopened database: %q, %v", name, err)
	}
	for _, table := range []string{"page", "wiki", "special_page", "image", "database_version"} {
		var found string
		if err := a.DB.QueryRow(`select name from sqlite_master where name = ?`, table).Scan(&found); err != nil {
			t.Fatalf("missing %s: %v", table, err)
		}
	}
}

func TestAllLegacySchemaMigrations(t *testing.T) {
	for existingVersion := 0; existingVersion <= len(migrations); existingVersion++ {
		t.Run(string(rune('0'+existingVersion)), func(t *testing.T) {
			storage := t.TempDir()
			db, err := sql.Open("sqlite", filepath.Join(storage, "archiver.db"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`create table database_version (version integer, primary key(version))`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`insert into database_version values (?)`, existingVersion); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < existingVersion; i++ {
				for _, statement := range migrations[i] {
					if _, err := db.Exec(statement); err != nil {
						t.Fatalf("seed migration %d: %v", i, err)
					}
				}
			}
			if existingVersion >= 1 {
				if _, err := db.Exec(`insert into wiki (wikiname, progress) values ('demo', 1)`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`insert into page (wikiname, basename, progress) values ('demo','Existing',1)`); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			a, err := Open(storage)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			var version int
			if err := a.DB.QueryRow(`select version from database_version`).Scan(&version); err != nil || version != 6 {
				t.Fatalf("migrated version %d, %v", version, err)
			}
			if existingVersion >= 1 {
				var progress int
				if err := a.DB.QueryRow(`select progress from wiki where wikiname='demo'`).Scan(&progress); err != nil {
					t.Fatal(err)
				}
				if existingVersion < 3 && progress != 2 {
					t.Errorf("legacy wiki stage should migrate to 2, got %d", progress)
				}
				if existingVersion >= 3 && progress != 1 {
					t.Errorf("wiki stage should remain 1, got %d", progress)
				}
			}
		})
	}
}
