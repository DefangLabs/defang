<script>
  import { onMount } from "svelte";
  let tasks = [];
  let newTaskTitle = "";

  onMount(async () => {
    try {
      const response = await fetch("/tasks");
      tasks = await response.json();
    } catch (error) {
      console.error("Failed to fetch tasks:", error);
    }
  });
  async function addTask() {
    if (!newTaskTitle.trim()) return;
    const response = await fetch("/tasks", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title: newTaskTitle, completed: false }),
    });
    if (response.ok) {
      const newTask = await response.json(); // Assuming your API returns the newly created task
      tasks = [...tasks, newTask]; // Add the new task to the local tasks array
      newTaskTitle = ""; // Clear the input field
    } else {
      console.error("Failed to add the task");
    }
  }

  async function updateTask(id, updatedTask) {
    console.log("@@ task details: ", id, updatedTask);
    const response = await fetch(`/tasks/${id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(updatedTask),
    });
    if (response.ok) {
      tasks = tasks.map((task) =>
        task.id === id ? { ...task, ...updatedTask } : task
      );
    }
  }

  async function toggleCompleted(task) {
    console.log("@@ task: ", task);
    const updatedTask = { ...task, completed: !task.completed };
    console.log("@@ task updated: ", updatedTask);
    if (await updateTask(task.id, updatedTask)) {
      // Manually update the task in the local state if the update was successful
      task.completed = !task.completed;
      console.log("clicked");
    }
  }

  async function deleteTask(taskId) {
    const response = await fetch(`/tasks/${taskId}`, {
      method: "DELETE",
    });
    if (response.ok) {
      tasks = tasks.filter((t) => t.id !== taskId);
    }
  }
</script>

<h1>Task Manager</h1>
<div>
  <input
    type="text"
    bind:value={newTaskTitle}
    placeholder="Enter new task..."
    on:keypress={(e) => e.key === "Enter" && addTask()}
  />
  <button on:click={addTask}>Add Task</button>
</div>

<ul>
  {#each tasks as task (task.id)}
    <!-- {@debug task} -->
    <li class:completed={task.completed}>
      <input
        type="checkbox"
        checked={task.completed}
        on:change={() => {
          console.log("@@ task in tmplt: ", task);
          toggleCompleted(task);
        }}
      />
      <input
        type="text"
        value={task.title}
        on:input={(event) =>
          updateTask(task.id, { ...task, title: event.target.value })}
      />
      <button on:click={() => deleteTask(task.id)}>Delete</button>
    </li>
  {/each}
</ul>

<style>
  .completed {
    text-decoration: line-through;
  }
  li {
    margin-bottom: 0.5rem;
  }
  input[type="text"],
  button {
    margin-right: 0.5rem;
  }
</style>
