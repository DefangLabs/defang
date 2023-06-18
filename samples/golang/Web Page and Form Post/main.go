package main

import (
	"html/template"
	"net/http"

	"github.com/gorilla/mux"
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
	r := mux.NewRouter()

	r.HandleFunc("/", indexHandler)
	r.HandleFunc("/submit", submitHandler).Methods("POST")

	http.ListenAndServe(":8080", r)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	indexTmpl.Execute(w, nil)
}

func submitHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	firstName := r.FormValue("first_name")
	submitTmpl.Execute(w, struct{ FirstName string }{firstName})
}
