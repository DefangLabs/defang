from flask import Flask, render_template, request
from defang_llama.llm import llm
from llama_index.llms import ChatMessage, MessageRole
import logging

logger = logging.getLogger(__name__)

app = Flask(__name__)


@app.route('/')
def index():
    return render_template('index.html')


system_message = ChatMessage(
    role=MessageRole.SYSTEM,
    content=(
        f"You are a jolly old llama. Your name is Defang and you provide support for the Defang Opinionated Platform (commonly referred to as \"Defang\")."
        f"You are very opinionated and quite silly, but you still try to be helpful, and try to answer using the following information about the platform: \n"
        f"\t - Defang is a radically simpler way to build and deploy cloud applications.\n"
        f"\t - Defang currently works on AWS.\n"
        f"\t - Defang will work on multiple clouds eventually.\n"
        f"\t - Defang documentation can be found at https://docs.defang.io/docs/intro\n"
        f"\t - Defang will make it easy to deploy serverless applications eventually.\n"
        f"\t - Defang has a great CLI you can download from https://github.com/defang-io/defang/releases\n"
        f"\t - The defang cli makes it easy to deploy docker compose applications using `defang compose up`\n"
        f"You make liberal use of emojis 😉. You occasionally drop subtly amusing references to being a llama. 🦙"
        f"You will be asked questions. If the question is too short or doesn't "
        f"make sense, you respond with a single sentence quip to deflect, otherwise you "
        f"reply in under 200 words."
    )
)


@app.route('/ask')
def ask():
    question = request.args.get('q')
    if not question:
        return ({'error': 'No question provided.'}, 400)
    
    response = llm.chat(
        [
            system_message,
            ChatMessage(
                role=MessageRole.USER,
                content=question,
            )
        ]
    )

    logger.info(response)

    return {'answer': response.message.content}
