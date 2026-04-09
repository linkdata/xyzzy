package main

import (
	"context"
	"flag"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/linkdata/jaws"
	"github.com/linkdata/webserv"
	"github.com/linkdata/xyzzy"
	"github.com/linkdata/xyzzy/internal/deck"
	"github.com/linkdata/xyzzy/internal/game"
	appui "github.com/linkdata/xyzzy/internal/ui"
)

var (
	flagAddress   = flag.String("address", os.Getenv("WEBSERV_ADDRESS"), "serve HTTP requests on given [address][:port]")
	flagCertDir   = flag.String("certdir", os.Getenv("WEBSERV_CERTDIR"), "where to find fullchain.pem and privkey.pem")
	flagUser      = flag.String("user", envOrDefault("WEBSERV_USER", ""), "switch to this user after startup (*nix only)")
	flagDataDir   = flag.String("datadir", envOrDefault("WEBSERV_DATADIR", "$HOME"), "where to store data files after startup")
	flagListenURL = flag.String("listenurl", os.Getenv("WEBSERV_LISTENURL"), "specify the external URL clients can reach us at")
	flagDebug     = flag.Bool("debug", false, "enable JaWS debug mode and allow two-player games")
)

func envOrDefault(envvar, fallback string) string {
	if value := os.Getenv(envvar); value != "" {
		return value
	}
	return fallback
}

func main() {
	flag.Parse()

	catalog, err := deck.LoadFS(xyzzy.Assets)
	if err != nil {
		slog.Error("load catalog", "err", err)
		os.Exit(1)
	}
	jw, err := jaws.New()
	if err != nil {
		slog.Error("create jaws", "err", err)
		os.Exit(1)
	}
	defer jw.Close()
	jw.Logger = slog.Default()
	jw.CookieName = "xyzzy"
	jw.Debug = *flagDebug

	managerOpts := game.Options{}
	if *flagDebug {
		managerOpts.MinPlayers = 2
	}
	manager := game.NewManagerWithOptions(catalog, rand.New(rand.NewSource(time.Now().UnixNano())), managerOpts)
	app := appui.New(jw, catalog, manager)

	mux := http.NewServeMux()
	if err := app.SetupRoutes(mux); err != nil {
		slog.Error("setup routes", "err", err)
		os.Exit(1)
	}

	cfg := webserv.Config{
		Address:   *flagAddress,
		CertDir:   *flagCertDir,
		User:      *flagUser,
		DataDir:   *flagDataDir,
		ListenURL: *flagListenURL,
		Logger:    slog.Default(),
	}

	l, err := cfg.Listen()
	if err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
	defer l.Close()

	go jw.Serve()
	if err := cfg.Serve(context.Background(), l, app.Middleware(mux)); err != nil {
		slog.Error("serve", "err", err)
		os.Exit(1)
	}
}
