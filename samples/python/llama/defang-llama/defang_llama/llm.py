import os
from llama_index.llms import Ollama

llm = Ollama(
    model=os.environ.get('MODEL', 'phi'),
    request_timeout=60,
)