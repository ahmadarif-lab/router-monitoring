// router-monitoring: a dashboard for your routers (Biznet/Huawei ONT, GL.iNet).
// One binary: background collectors + scheduled restart + web UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ahmadarif-lab/router-monitoring/internal/collector"
	"github.com/ahmadarif-lab/router-monitoring/internal/config"
	"github.com/ahmadarif-lab/router-monitoring/internal/drivers"
	"github.com/ahmadarif-lab/router-monitoring/internal/restart"
	"github.com/ahmadarif-lab/router-monitoring/internal/server"
)

// version is set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func defaultDataDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "router-monitoring")
	}
	return "router-monitoring-data"
}

// demoProfiles are the two made-up routers `-demo` starts with.
func demoProfiles() []config.Profile {
	return []config.Profile{
		{Name: "Office (GL.iNet)", Type: drivers.DemoGLiNet, URL: "http://demo.invalid", User: "demo", Pass: "demo"},
		{Name: "Home (Biznet)", Type: drivers.DemoHuawei, URL: "http://demo.invalid", User: "demo", Pass: "demo"},
	}
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8787", "HTTP listen address")
	data := flag.String("data", defaultDataDir(), "data folder (profiles.json, aliases.json, history)")
	hosts := flag.String("host", "", "extra allowed host names, comma separated (e.g. router.lan)")
	lan := flag.Bool("lan", false, "allow other devices on the network (there is no login: trusted networks only)")
	demo := flag.Bool("demo", false, "run with invented data and two sample profiles (nothing is saved)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("router-monitoring", version)
		return
	}
	log.SetFlags(log.Ldate | log.Ltime)

	if *demo { // throw-away data folder with two made-up routers
		tmp, err := os.MkdirTemp("", "router-monitoring-demo-")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(tmp)
		*data = tmp
	}
	store, err := config.OpenStore(*data)
	if err != nil {
		log.Fatalf("cannot open data folder %s: %v", *data, err)
	}
	if *demo {
		var first string
		for _, p := range demoProfiles() {
			saved, err := store.SaveProfile(p)
			if err != nil {
				log.Fatal(err)
			}
			if first == "" {
				first = saved.ID
			}
		}
		_ = store.SetActive(first)
	}
	mgr := collector.NewManager(store)
	rst := restart.NewRestarter(store, mgr)
	srv := server.New(store, mgr, rst, strings.Split(*hosts, ","), *lan, version)

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("cannot listen on %s: %v (port in use? try -listen 127.0.0.1:8788)", *listen, err)
	}
	httpSrv := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopSched := make(chan struct{})
	mgr.Start(ctx)
	go rst.RunSchedule(stopSched)

	log.Printf("router-monitoring %s listening on http://%s (data: %s)", version, *listen, *data)
	if len(store.Profiles()) == 0 {
		log.Printf("no router configured yet: open the address above and add one")
	}
	go func() {
		<-ctx.Done()
		close(stopSched)
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(sctx)
	}()
	if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	mgr.Stop() // waits for the collector to save its state
}
