import os
import json
import openai
from flask import Flask
from flask import request


app = Flask(__name__)


@app.route("/")
def index():
    return { 'status' : 'ok' }


@app.route("/prompt", methods=['POST'])
def prompt():
    openai.api_key = os.environ['OPENAI_KEY']

    messages = [ 
        # System prompt used to set context for the conversation
        # { "role": "system", "content": "You are an experienced software engineer." }
    ]

    prompt_text = request.get_data()
    prompt_text = prompt_text.decode('utf-8')

    # append user prompt from request
    messages.append({ "role": "user", "content": prompt_text })

    # call the ChatGPT API
    response = openai.ChatCompletion.create(
        model="gpt-3.5-turbo",
        #engine="gpt-4",
        messages=messages,
    )

    return { 'status' : 'ok', 'response' : response }
        

if __name__ == '__main__':
    app.run(debug=True, host='0.0.0.0')

