# Use ubuntu:20.04 to download the model
FROM ubuntu:20.04 AS downloader

RUN apt-get update && \
    apt-get install -y curl && \
    rm -rf /var/lib/apt/lists/*

ARG MODEL_FILE=llama-2-7b-chat.Q4_0.gguf
ARG MODEL_URL=https://huggingface.co/TheBloke/Llama-2-7B-chat-GGUF/resolve/main/llama-2-7b-chat.Q4_0.gguf

WORKDIR /models
RUN curl -L $MODEL_URL -o "./${MODEL_FILE}"

# Use python:3.9-slim for the application
FROM python:3.9-slim

# Copy the downloaded model from the downloader stage
COPY --from=downloader /models /models

# Install necessary build dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends build-essential git xxd && \
    rm -rf /var/lib/apt/lists/*

# Create and switch to user 'defang-llama'
RUN useradd -m defang-llama
USER defang-llama

# Install poetry as the user 'defang-llama'
RUN pip install --user poetry

# Set PATH environment variable to include user's local bin
ENV PATH="/home/defang-llama/.local/bin:$PATH"

# Set the working directory
WORKDIR /app

# (Optional) Copy your Python project files and install dependencies
COPY --chown=defang-llama:defang-llama ./defang-llama/ /app/

RUN CMAKE_ARGS="-DLLAMA_CUBLAS=on" poetry install --no-dev

ARG MODEL_FILE=llama-2-7b-chat.Q4_0.gguf

ENV MODEL_PATH="/models/${MODEL_FILE}"

ENTRYPOINT [ "poetry" ]

CMD ["run", "flask", "--app", "defang_llama", "run", "--host", "0.0.0.0"]
