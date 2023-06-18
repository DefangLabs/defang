package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
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

		if resp.StatusCode != 200 {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(Response{Status: "error"})
			return
		}

		body, _ := ioutil.ReadAll(resp.Body)

		var data interface{}
		json.Unmarshal(body, &data)

		json.NewEncoder(w).Encode(data)
	})

	fmt.Println("Starting server at port 8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
