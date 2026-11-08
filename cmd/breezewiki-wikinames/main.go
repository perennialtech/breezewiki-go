package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/perennialtech/breezewiki-go/internal/fandomhttp"
	"github.com/perennialtech/breezewiki-go/internal/wikinames"
)

func main() {
	client, err := fandomhttp.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	entries, err := wikinames.Download(context.Background(), client, "https://community.fandom.com/wiki/Special:NewWikis", func(offset string, count, total int) {
		fmt.Printf("page offset %q has %d items (%d so far)\n", offset, count, total)
	})
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.Create("wiki-names.json")
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(f).Encode(entries); err != nil {
		f.Close()
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}
