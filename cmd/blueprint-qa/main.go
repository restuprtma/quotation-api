package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"
	"venturo-skeleton-go/internal/config"
	"venturo-skeleton-go/internal/modules/quotation/qa"
)

func main() {
	_ = godotenv.Load(".env")
	cfg := config.Load()
	pool, err := pgxpool.New(context.Background(), cfg.Database.GetDSN())
	if err != nil {
		log.Fatal("QA database initialization failed")
	}
	defer pool.Close()
	if err = pool.Ping(context.Background()); err != nil {
		log.Fatal("QA database unavailable")
	}
	fixture := filepath.Join("internal", "modules", "quotation", "service", "testdata", "blueprint_http", "queue.json")
	srv, err := qa.NewServer(pool, fixture)
	if err != nil {
		log.Fatal("QA seed failed: ", err)
	}
	defer srv.Close()
	port := os.Getenv("BLUEPRINT_QA_PORT")
	if port == "" {
		port = "18080"
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		log.Fatal("QA listen failed: ", err)
	}
	httpServer := &http.Server{Handler: srv.Handler()}
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Print("QA server stopped")
		}
	}()
	log.Printf("blueprint QA ready on http://127.0.0.1:%s", port)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}
