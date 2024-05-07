package main

import (
	"html/template"
	"log"
	"net/http"
	"strconv"
	"sync"
)

// Task represents a single task in the task list
type Task struct {
	Content string
}

// TaskList holds all tasks in memory
var (
	taskList []Task
	mu       sync.Mutex
)

// Define templates with embedded HTML and CSS
var indexTmpl = template.Must(template.New("index").Parse(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Task Manager</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        #task-app { width: 300px; margin: auto; padding: 20px; border: 1px solid #ccc; border-radius: 5px; }
        input[type="text"], input[type="submit"] { width: calc(100% - 22px); padding: 10px; margin-bottom: 10px; }
        ul { list-style: none; padding: 0; }
        li { margin: 10px 0; padding: 10px; background-color: #f9f9f9; border: 1px solid #e1e1e1; position: relative; }
        .delete-btn { position: absolute; top: 50%; right: 10px; transform: translateY(-50%); }
        .delete-btn input[type="submit"] { background: none; border: none; cursor: pointer; }
    </style>
</head>
<body>
    <div id="task-app">
        <h1>Task Manager</h1>
        <form action="/add-task" method="post">
            <input type="text" name="task" placeholder="Add a new task">
            <input type="submit" value="Add Task">
        </form>
        <ul>
            {{range $index, $task := .}}
                <li>
                    {{.Content}}
                    <form action="/remove-task" method="post" class="delete-btn">
                        <input type="hidden" name="index" value="{{$index}}">
                        <input type="submit" value="-">
                    </form>
                </li>
            {{else}}
                <li>No tasks yet!</li>
            {{end}}
        </ul>
    </div>
</body>
</html>
`))

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/add-task", addTaskHandler)
	http.HandleFunc("/remove-task", removeTaskHandler)

	log.Println("Server starting on http://localhost:8080/")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	if err := indexTmpl.Execute(w, taskList); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func addTaskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	mu.Lock()
	defer mu.Unlock()
	taskContent := r.FormValue("task")
	if taskContent != "" {
		taskList = append(taskList, Task{Content: taskContent})
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func removeTaskHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	index := r.FormValue("index")
	if index == "" {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}

	i, err := strconv.Atoi(index)
	if err != nil || i < 0 || i >= len(taskList) {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}

	taskList = append(taskList[:i], taskList[i+1:]...)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
