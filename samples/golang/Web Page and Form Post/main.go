package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

var indexTmpl = template.Must(template.New("index").Parse(`
<!DOCTYPE html>
<html>
<head>
    <title>Simple form post</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 20px;
        }

        label {
            font-weight: bold;
        }

        input[type="text"] {
            padding: 5px;
            margin-bottom: 10px;
            width: 200px;
        }

        input[type="submit"] {
            padding: 10px 20px;
            background-color: #4CAF50;
            color: white;
            border: none;
            cursor: pointer;
        }
    </style>
</head>
<body>
    <h1>Simple form post</h1>
    <form action="/submit" method="post">
        <label for="first_name">First name:</label><br>
        <input type="text" id="first_name" name="first_name"><br>
        <input type="submit" value="Submit">
    </form>
</body>
</html>
`))

var submitTmpl = template.Must(template.New("submit").Parse(`
    <html>
        <head>
            <title>Simple form post</title>
        </head>
        <body>
            <h1>Hello {{.FirstName}}!</h1>
        </body>
    </html>
`))

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/submit", submitHandler)

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

func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	indexTmpl.Execute(w, nil)
}

func submitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	firstName := r.FormValue("first_name")
	submitTmpl.Execute(w, struct{ FirstName string }{firstName})
}
