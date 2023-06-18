

from flask import Flask
from flask import request


app = Flask(__name__)


@app.route("/")
def index():
    # Set response header to HTML
    headers = {
        "Content-Type": "text/html",
    }

    # Declare the HTML to return via the response
    content = """
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
    """

    # Return the HTML
    return content, 200, headers
        


@app.route("/submit", methods=["POST"])
def submit():
    # Set response header to HTML
    headers = {
        "Content-Type": "text/html",
    }

    # Get the form data
    first_name = request.form.get('first_name')

    # Declare the HTML to return via the response
    content = """
    <html>
        <head>
            <title>Simple form post</title>
        </head>
        <body>
            <h1>Hello {}!</h1>
        </body>
    </html>
    """.format(first_name)

    # Return the HTML
    return content, 200, headers
    

if __name__ == '__main__':
    app.run(debug=True, host='0.0.0.0')
