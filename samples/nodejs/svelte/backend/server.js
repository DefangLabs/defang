const express = require('express');
const mysql = require('mysql2');
const cors = require('cors');
const path = require('path');

const app = express();
app.use(cors());
app.use(express.json());

// Create a pool of connections
const pool = mysql.createPool({
    host: process.env.DATABASE_HOST,
    user: process.env.DATABASE_USER,
    password: process.env.DATABASE_PASSWORD,
    database: process.env.DATABASE_NAME,
    waitForConnections: true,
    connectionLimit: 10,
    queueLimit: 0
});

// Function to ensure 'tasks' table exists
const ensureTasksTable = () => {
    const sql = `
        CREATE TABLE IF NOT EXISTS tasks (
            id INT AUTO_INCREMENT PRIMARY KEY,
            title VARCHAR(255) NOT NULL,
            completed BOOLEAN NOT NULL DEFAULT FALSE
        );
    `;
    pool.query(sql, (error, results) => {
        if (error) {
            console.error('Error ensuring tasks table exists:', error);
            return;
        }
        console.log('Tasks table checked/created.');
    });
};

// Ensure the tasks table is created before starting the server
pool.getConnection((err, connection) => {
    if (err) {
        console.error('Error connecting to the database:', err);
        process.exit(1);
    } else {
        console.log('Connected to the database.');
        ensureTasksTable();
        connection.release();

        // Start the server only after ensuring the table exists
        const PORT = process.env.PORT || 3001;
        app.listen(PORT, () => {
            console.log(`Server running on port ${PORT}`);
        });
    }
});

// API Routes
app.get('/tasks', (req, res) => {
    pool.query('SELECT * FROM tasks', (error, results) => {
        if (error) {
            console.error(error);
            return res.status(500).json({ error: "Internal server error" });
        }
        res.json(results);
    });
});

app.post('/tasks', (req, res) => {
    const { title } = req.body;
    pool.query('INSERT INTO tasks SET ?', { title, completed: false }, (error, results) => {
        if (error) {
            console.error(error);
            return res.status(500).json({ error: "Internal server error" });
        }
        res.status(201).json({ id: results.insertId, title, completed: false });
    });
});

app.put('/tasks/:id', (req, res) => {
    const { id } = req.params;
    const { title, completed } = req.body;
    pool.query(
        'UPDATE tasks SET title = ?, completed = ? WHERE id = ?',
        [title, completed, id],
        (error, results) => {
            if (error) {
                console.error(error);
                return res.status(500).json({ error: "Internal server error" });
            }
            if (results.affectedRows === 0) {
                return res.status(404).json({ error: "Task not found" });
            }
            res.json({ id, title, completed });
        }
    );
});

app.delete('/tasks/:id', (req, res) => {
    const { id } = req.params;
    pool.query('DELETE FROM tasks WHERE id = ?', [id], (error, results) => {
        if (error) {
            console.error(error);
            return res.status(500).json({ error: "Internal server error" });
        }
        res.send('Task deleted.');
    });
});

// Serve static files from the "public" directory
app.use(express.static(path.join(__dirname, 'public')));

// Serve the frontend from the root
app.get('*', (req, res) => {
    res.sendFile(path.join(__dirname, 'public', 'index.html'));
});
