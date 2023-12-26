FROM nvidia/cuda:12.3.1-devel-ubuntu20.04

# Install necessary build dependencies
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends software-properties-common && \
    add-apt-repository ppa:deadsnakes/ppa && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl build-essential git xxd python3.9 python3-pip && \
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

ARG MODEL_FILE=llama-2-7b-chat.Q4_0.gguf
ARG MODEL_URL=https://huggingface.co/TheBloke/Llama-2-7B-chat-GGUF/resolve/main/${MODEL_FILE}
ENV MODEL_PATH="/app/models/${MODEL_FILE}"
RUN mkdir -p /app/models && curl -L ${MODEL_URL} -o ${MODEL_PATH}
# ADD ${MODEL_URL} /app/models/ # should work but OOM

# (Optional) Copy your Python project files and install dependencies
COPY ./defang-llama/pyproject.toml ./defang-llama/README.md ./defang-llama/poetry.lock ./defang-llama/poetry.toml /app/

ENV CMAKE_ARGS="-DLLAMA_CUBLAS=on"

RUN poetry install --only main --no-root

ENV FLASK_APP=defang_llama
ENV FLASK_ENV=production
ENV FLASK_RUN_HOST=0.0.0.0
ENV FLASK_RUN_PORT=5000

COPY ./defang-llama/ /app/

# TODO: nividia-smi

ENTRYPOINT [ "./run.sh" ]
