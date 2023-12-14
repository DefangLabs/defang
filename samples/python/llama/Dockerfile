# Use python:3.9-slim for the application
FROM python:3.9-slim

# Install necessary build dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends curl build-essential git xxd && \
    rm -rf /var/lib/apt/lists/*

# Create and switch to user 'defang-llama'
# RUN useradd -m defang-llama
# USER defang-llama

# Install poetry as the user 'defang-llama'
# RUN pip install --user poetry
RUN pip install poetry

# Set PATH environment variable to include user's local bin
# ENV PATH="/home/defang-llama/.local/bin:$PATH"

# Set the working directory
WORKDIR /app

# (Optional) Copy your Python project files and install dependencies
COPY ./defang-llama/pyproject.toml ./defang-llama/poetry.lock ./defang-llama/poetry.toml /app/

RUN CMAKE_ARGS="-DLLAMA_CUBLAS=on" poetry install --no-dev
# RUN poetry install --no-dev

ARG MODEL_FILE=llama-2-7b-chat.Q4_0.gguf
ARG MODEL_URL=https://huggingface.co/TheBloke/Llama-2-7B-chat-GGUF/resolve/main/llama-2-7b-chat.Q4_0.gguf

ENV MODEL_FILE=${MODEL_FILE}
ENV MODEL_URL=${MODEL_URL}
ENV MODEL_PATH="/app/models/${MODEL_FILE}"
ENV FLASK_APP=defang_llama
ENV FLASK_ENV=production
ENV FLASK_RUN_HOST=0.0.0.0
ENV FLASK_RUN_PORT=5000

COPY ./defang-llama/ /app/

ENTRYPOINT [ "./run.sh" ]
