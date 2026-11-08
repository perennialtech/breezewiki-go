package server

import (
	"database/sql"
	"errors"
	"os"

	_ "modernc.org/sqlite"
)

// OpenArchiveDatabase attaches an existing archiver database. The archive
// tool creates and migrates its schema; the HTTP server only reads it.
// A missing database is a valid state for an unarchived installation.
func (s *Server) OpenArchiveDatabase(filename string) error {
	_, err := os.Stat(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", "file:"+filename+"?mode=ro")
	if err != nil {
		return err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return err
	}
	s.DB = db
	return nil
}
func (s *Server) CloseArchiveDatabase() error {
	if s.DB == nil {
		return nil
	}
	return s.DB.Close()
}
