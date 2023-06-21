package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

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
			http.Error(w, "Invalid method", 405)
			return
		}

		var data map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&data)
		if err != nil {
			http.Error(w, "Invalid JSON", 400)
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
			http.Error(w, "Invalid method", 405)
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
				http.Error(w, "File not found in S3 bucket", 404)
			} else {
				// return the exact error
				http.Error(w, err.Error(), 500)

				//http.Error(w, "Unknown error", 500)
			}
			return
		}

		var fileContent map[string]interface{}
		err = json.NewDecoder(res.Body).Decode(&fileContent)
		if err != nil {
			http.Error(w, "Failed to decode file content", 500)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fileContent)
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
