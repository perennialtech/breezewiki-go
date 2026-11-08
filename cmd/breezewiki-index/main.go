// Command breezewiki-index extracts offline pages into a Solr full-text index.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/perennialtech/breezewiki-go/internal/archive"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	flags := flag.NewFlagSet("breezewiki-index", flag.ContinueOnError)
	cached := flags.Bool("read-from-cache", false, "read documents from the previous index cache instead of rebuilding")
	flags.BoolVar(cached, "c", false, "read documents from the previous index cache")
	cache := flags.String("cache", "cache.rkt", "index document cache file")
	storage := flags.String("storage", "storage", "directory with archive/")
	solr := flags.String("solr-url", "http://localhost:8983/solr", "Solr base URL")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: breezewiki-index [flags] <wikiname>")
		flags.PrintDefaults()
	}
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 1 || !archive.ValidWiki(flags.Arg(0)) {
		flags.Usage()
		return fmt.Errorf("please specify a valid wikiname")
	}
	a, err := archive.Open(*storage)
	if err != nil {
		return err
	}
	defer a.Close()
	started := time.Now()
	return a.Index(context.Background(), flags.Arg(0), archive.IndexOptions{ReadFromCache: *cached, CachePath: *cache, SolrURL: *solr}, func(done, total int, name string) {
		seconds := time.Since(started).Seconds()
		if seconds == 0 {
			seconds = 0.001
		}
		rate := float64(done) / seconds
		percent, eta := 100, 0
		if total > 0 {
			percent = done * 100 / total
			if rate > 0 {
				eta = int(float64(total-done) / rate)
			}
		}
		filled := percent * 34 / 100
		if filled < 0 {
			filled = 0
		}
		if filled > 34 {
			filled = 34
		}
		nameChars := []rune(name)
		if len(nameChars) > 30 {
			name = string(nameChars[:27]) + "..."
		}
		fmt.Printf("\033[2K\r%d/%d %.0f/s %d:%02d %d%% [%s%s] %s", done, total, rate, eta/60, eta%60, percent, strings.Repeat("=", filled), strings.Repeat(" ", 34-filled), name)
		if done == total {
			fmt.Println()
		}
	})
}
