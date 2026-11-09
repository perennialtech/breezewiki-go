package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/perennialtech/breezewiki-go/internal/config"
	"github.com/perennialtech/breezewiki-go/internal/fandom"
	"github.com/perennialtech/breezewiki-go/internal/fandomhttp"
	"github.com/perennialtech/breezewiki-go/internal/server"
)

func run() error {
	var configFile, root string
	flag.StringVar(&configFile, "config", "config.ini", "path to config.ini (BW_ environment variables override it)")
	flag.StringVar(&root, "root", ".", "directory containing static/")
	flag.Parse()
	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(cfg.Get("port"))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port %q", cfg.Get("port"))
	}
	host := cfg.Get("bind_host")
	if host == "auto" {
		if cfg.True("debug") {
			host = "127.0.0.1"
		} else {
			host = ""
		}
	}
	client, err := fandomhttp.NewClient()
	if err != nil {
		return err
	}
	f := fandom.New(cfg)
	f.HTTP = client
	s := server.New(cfg, f)
	if info, err := os.Stat(filepath.Join(root, "static")); err == nil && info.IsDir() {
		s.StaticDir = filepath.Join(root, "static")
	}
	srv := &http.Server{Addr: net.JoinHostPort(host, strconv.Itoa(port)), Handler: s, ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Printf("%s listening at %s", cfg.Get("application_name"), srv.Addr)
	err = srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
