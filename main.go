package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/realmroot/lightkite/pkg/common"
	"github.com/realmroot/lightkite/pkg/version"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	appCtx, cancelApp := context.WithCancel(context.Background())

	app, err := initializeApp(appCtx)
	if err != nil {
		cancelApp()
		log.Fatalf("Failed to initialize app: %v", err)
	}
	pprofServer, err := startPprofServer(common.PprofAddress)
	if err != nil {
		cancelApp()
		log.Fatalf("Failed to start pprof server: %v", err)
	}
	defer cancelApp()

	srv := &http.Server{
		Addr:              ":" + common.Port,
		Handler:           buildEngine(app).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			klog.Fatalf("Failed to start server: %v", err)
		}
	}()
	klog.Infof("Lightkite server started on port %s", common.Port)
	klog.Infof("Version: %s, Build Date: %s, Commit: %s",
		version.Version, version.BuildDate, version.CommitID)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	klog.Info("Shutting down server...")
	app.ready.Store(false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		klog.Fatalf("Failed to shutdown server: %v", err)
	}
	if pprofServer != nil {
		if err := pprofServer.Shutdown(ctx); err != nil {
			klog.Errorf("Failed to shutdown pprof server: %v", err)
		}
	}
	cancelApp()
}

func startPprofServer(address string) (*http.Server, error) {
	if address == "" {
		return nil, nil
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	server := &http.Server{
		Addr:              address,
		Handler:           http.DefaultServeMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			klog.Errorf("pprof server stopped: %v", err)
		}
	}()
	klog.Infof("pprof server started on %s", listener.Addr())
	return server, nil
}
