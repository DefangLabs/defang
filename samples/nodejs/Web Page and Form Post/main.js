const express = require('express');
const bodyParser = require('body-parser');

const app = express();

// to support URL-encoded bodies
app.use(bodyParser.urlencoded({ extended: true }));

app.get('/', (req, res) => {
    let html = `
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
    </html>`;
    res.send(html);
});

app.post('/submit', (req, res) => {
    let firstName = req.body.first_name;
    let html = `
    <html>
        <head>
            <title>Simple form post</title>
        </head>
        <body>
            <h1>Hello ${firstName}!</h1>
        </body>
    </html>`;
    res.send(html);
});

const PORT = 3000;

app.listen(PORT, () => {
    console.log(`Server is running on port ${PORT}.`);
});
