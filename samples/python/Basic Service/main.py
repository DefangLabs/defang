from flask import Flask
from flask import request


app = Flask(__name__)


@app.route("/", methods=['GET', 'POST'])
def index():
    return  {
        "path" : request.path,
        "method" : request.method,
        "headers" : dict(request.headers),
        "args" : dict(request.args),
        "body" : request.data.decode('utf-8')
    }


if __name__ == '__main__':
    app.run(debug=True, host='0.0.0.0')
