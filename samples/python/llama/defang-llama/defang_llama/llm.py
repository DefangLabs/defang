import os
from llama_index.llms import Vllm

os.environ["HF_HOME"] = "model/"

llm = Vllm(
    model=""
)