// Command breezewiki-archiver downloads one Fandom wiki to the legacy offline format.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/perennialtech/breezewiki-go/internal/archive"
	"golang.org/x/term"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("breezewiki-archiver", flag.ContinueOnError)
	quiet := flags.Bool("output-quiet", false, "disable progress output")
	lines := flags.Bool("output-lines", false, "print each downloaded filename (default outside a terminal)")
	progress := flags.Bool("output-progress", false, "in-place terminal progress (default in a terminal)")
	flags.BoolVar(quiet, "q", false, "disable progress output")
	flags.BoolVar(lines, "l", false, "print each downloaded filename")
	flags.BoolVar(progress, "p", false, "in-place terminal progress")
	storage := flags.String("storage", "storage", "directory for archiver.db and archive/")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: breezewiki-archiver [flags] <wikiname>")
		fmt.Fprintln(flags.Output(), "Download a Fandom wiki into storage/archive/. The SQLite database in storage/archiver.db records download progress for resuming interrupted runs.")
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
	if toInt(*quiet)+toInt(*lines)+toInt(*progress) > 1 {
		return fmt.Errorf("-q, -p and -l are mutually exclusive")
	}
	if !*quiet && !*lines && !*progress {
		if isTerminal() {
			*progress = true
		} else {
			*lines = true
		}
	}
	a, err := archive.Open(*storage)
	if err != nil {
		return err
	}
	defer a.Close()
	wiki := flags.Arg(0)
	callback := func(done, total int, name string) {
		switch {
		case *lines:
			if name != "" {
				if decoded, err := archive.BasenameTitle(name); err == nil {
					name = decoded
				}
				fmt.Println(name)
			}
		case *progress:
			prefix := fmt.Sprintf("[%s] [%d/%d] ", wiki, done, total)
			width := terminalWidth()
			name = strings.ReplaceAll(name, "\n", " ")
			if avail := width - len(prefix); avail < len([]rune(name)) {
				if avail < 0 {
					avail = 0
				}
				name = string([]rune(name)[:avail])
			}
			fmt.Printf("\033[2K\r%s%s", prefix, name)
		}
	}
	for i := range archive.StageNames {
		fmt.Printf("> Stage %d/%d\n", i+1, len(archive.StageNames))
		if err := a.RunStage(context.Background(), wiki, i, callback); err != nil {
			return err
		}
		fmt.Println()
	}
	return nil
}

func toInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
func isTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
func terminalWidth() int {
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 0 {
		return n
	}
	return 80
}
