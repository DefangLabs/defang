# Run Llama on Defang

Get started on Mac...

First make sure you're in the `defang-llama` dir.

Then...
```
# Download the model if it's not already there
if [ ! -f "models/llama-2-7b-chat.Q4_0.gguf" ]; then
    curl -L https://huggingface.co/TheBloke/Llama-2-7B-chat-GGUF/resolve/main/llama-2-7b-chat.Q4_0.gguf -o "models/llama-2-7b-chat.Q4_0.gguf"
fi

# Install the dependencies
CMAKE_ARGS="-DLLAMA_METAL=on" poetry install

# Run the server
MODEL_PATH="$(pwd)/models/llama-2-7b-chat.Q4_0.gguf" FLASK_ENV=development poetry run flask --app defang_llama run --debug 
```