package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var (
	REGION_NAME = "us-west-2"
	BUCKET_NAME = "my-sample-bucket"
	FILE_NAME   = "file1.json"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
			return
		}

		var data map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(REGION_NAME))
		if err != nil {
			log.Fatalf("unable to load SDK config, %v", err)
		}

		client := s3.NewFromConfig(cfg)

		jsonData, err := json.Marshal(data)
		if err != nil {
			log.Fatalf("failed to encode data to JSON, %v", err)
		}

		input := &s3.PutObjectInput{
			Bucket: &BUCKET_NAME,
			Key:    &FILE_NAME,
			Body:   bytes.NewReader(jsonData),
		}

		_, err = client.PutObject(context.TODO(), input)

		if err != nil {
			log.Fatalf("failed to upload object, %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	http.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
			return
		}

		cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(REGION_NAME))
		if err != nil {
			log.Fatalf("unable to load SDK config, %v", err)
		}

		client := s3.NewFromConfig(cfg)

		res, err := client.GetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: &BUCKET_NAME,
			Key:    &FILE_NAME,
		})

		if err != nil {
			var nfe *types.NoSuchKey
			if errors.As(err, &nfe) {
				http.Error(w, "File not found in S3 bucket", http.StatusNotFound)
			} else {
				// return the exact error
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		var fileContent map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&fileContent)
		if err != nil {
			http.Error(w, "Failed to decode file content", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fileContent)
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
