package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ikitiki/postgresql_exporter/pkg/config"
	"github.com/ikitiki/postgresql_exporter/pkg/pgcollector"
)

const (
	indexHTML = `
<html>
	<head>
		<title>Postgresql Exporter</title>
	</head>
	<body>
		<h1>Postgresql Exporter</h1>
		<p>
			<a href='%s'>Metrics</a>
		</p>
	</body>
</html>
`
)

var (
	version string

	showVersion   = flag.Bool("version", false, "output version information, then exit")
	configFile    = flag.String("config", "config.yaml", "path to the config file")
	metricsPath   = flag.String("web.telemetry-path", "/metrics", "path under which to expose metrics")
	listenAddress = flag.String("web.listen-address", ":9187", "address to listen on for web interface and telemetry")
)

func init() {
	flag.Parse()
}

func main() {
	if *showVersion {
		fmt.Printf("postgresql prometheus exporter %s", version)
		os.Exit(0)
	}

	cfg := config.New(*configFile)
	if err := cfg.Load(); err != nil {
		log.Fatalf("could not load config: %v", err)
	}
	collector := pgcollector.New()
	collector.LoadConfig(cfg)

	if err := prometheus.Register(collector); err != nil {
		log.Fatalf("could not register collector: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(fmt.Sprintf(indexHTML, *metricsPath)))
	})
	mux.Handle(*metricsPath, promhttp.Handler())

	srv := http.Server{
		Addr:    *listenAddress,
		Handler: mux,
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)

	log.Printf("starting postgresql exporter: %s", srv.Addr)
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("could not start http server: %v", err)
		}
	}()

loop:
	for {
		switch sig := <-sigs; sig {
		case syscall.SIGINT:
			fallthrough
		case syscall.SIGTERM:
			break loop
		case syscall.SIGHUP:
			log.Printf("reloading config. to be implemented")
		default:
			log.Printf("received signal: %v", sig)
		}
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Printf("could not shutdown http server: %v", err)
	}

	close(sigs)
	os.Exit(1)
}
