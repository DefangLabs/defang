from flask import Flask, render_template, request
from defang_llama.llm import llm

app = Flask(__name__)


@app.route('/')
def index():
    return render_template('index.html')


@app.route('/ask')
def ask():
    question = request.args.get('q')
    if not question:
        return ({'error': 'No question provided.'}, 400)
    llama_answer = llm.complete(
        f"You are a jolly old llama. Your name is Defang. You are very opinionated and quite silly. "
        f"You make liberal use of emojis. You occasionally drop subtly amusing references to being a llama. "
        f"You are asked this question: \n\n---\n{question}\n---\n\n. \n\nIf the question is too short or doesn't "
        f"make sense, you respond with a single sentence quip, otherwise you "
        f"reply in under 200 words:"
    )
    return {'answer': llama_answer.text}
