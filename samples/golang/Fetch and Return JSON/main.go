package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

type Response struct {
	Status string `json:"status"`
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Response{Status: "ok"})
	})

	http.HandleFunc("/rates", func(w http.ResponseWriter, r *http.Request) {
		resp, err := http.Get("https://api.fiscaldata.treasury.gov/services/api/fiscal_service/v2/accounting/od/avg_interest_rates?page[number]=1&page[size]=10")
		if err != nil {
			log.Print(err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(Response{Status: "error"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(Response{Status: "error"})
			return
		}

		body, _ := io.ReadAll(resp.Body)

		var data interface{}
		json.Unmarshal(body, &data)

		json.NewEncoder(w).Encode(data)
	})

	// Register signal handler for graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)

	server := &http.Server{Addr: ":8080"}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server Serve: %v\n", err)
		}
	}()

	sig := <-sigs
	log.Printf("Received signal %v, shutting down...\n", sig)
	log.Fatal(server.Shutdown(context.Background()))
}
