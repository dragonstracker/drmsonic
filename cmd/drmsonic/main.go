package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"drmsonic/internal/config"
	"drmsonic/internal/state"
	"drmsonic/internal/web"
)

func main() {
	var (
		port        = flag.Int("p", 1234, "Http port to listen to.")
		bind        = flag.String("b", "0.0.0.0", "Http bind adress.")
		baseURL     = flag.String("baseurl", "", "Http base URL.")
		providers   = flag.String("providers", "providers", "Providers path, default is 'providers/'.")
		confFile    = flag.String("c", "drmsonic.json", "Config file.")
		user        = flag.String("user", "", "Static admin user.")
		password    = flag.String("password", "", "Static admin password.")
		allow       = flag.String("allow", "", "List of no-auth admin source ip addresses.")
		logFile     = flag.String("l", "", "Log file")
		verbose     = flag.Int("v", 0, "Verbose logs (1=verbose, 2=debug, 3=trace).")
		noautostart = flag.Bool("noautostart", false, "Disable autostart.")
		accessLog   = flag.Bool("accesslog", false, "Log every http request.")
		version     = flag.Bool("version", false, "Print version and exit.")
		path        = flag.String("path", "", "Working directory. All subpath will be created there.")
	)
	flag.Parse()

	if *version {
		fmt.Printf("%s %s\n", config.VersionName, config.VersionTag)
		return
	}

	if *path != "" {
		if err := os.Chdir(*path); err != nil {
			log.Fatalf("chdir %s: %v", *path, err)
		}
	}

	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err == nil {
			log.SetOutput(f)
			defer f.Close()
		}
	}
	if *verbose == 0 {
		log.SetFlags(0)
	}

	store, err := config.NewStore(*providers, *confFile)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	mgr := state.NewManager()

	opts := web.Options{
		Bind:        *bind,
		Port:        *port,
		BaseURL:     *baseURL,
		Allow:       *allow,
		User:        *user,
		Password:    *password,
		Providers:   *providers,
		NoAutostart: *noautostart,
		AccessLog:   *accessLog,
	}

	srv, err := web.New(opts, store, mgr)
	if err != nil {
		log.Fatalf("web: %v", err)
	}

	if !*noautostart {
		srv.Autostart()
	}

	go func() {
		addr := fmt.Sprintf("%s:%d", *bind, *port)
		log.Printf("%s %s listening on http://%s", config.VersionName, config.VersionTag, addr)
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sig:
		log.Printf("signal received, stopping")
	case <-srv.ShutdownSignal():
		log.Printf("shutdown requested from panel")
	}
	srv.StopAll()
}
