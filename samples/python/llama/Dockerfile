FROM nvidia/cuda:12.3.1-devel-ubuntu22.04

# Install necessary build dependencies
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends software-properties-common && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends software-properties-common && \
    add-apt-repository ppa:deadsnakes/ppa && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl build-essential git xxd python3.9 python3.9-dev python3-pip openssh-server && \
    rm -rf /var/lib/apt/lists/*

# Configure SSH
RUN mkdir /var/run/sshd \
    && echo 'root:Potatoes!1' | chpasswd \
    && sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes\nPermitEmptyPasswords yes\nPasswordAuthentication yes/' /etc/ssh/sshd_config

# Install ngrok
RUN  curl -s https://ngrok-agent.s3.amazonaws.com/ngrok.asc | tee /etc/apt/trusted.gpg.d/ngrok.asc >/dev/null && echo "deb https://ngrok-agent.s3.amazonaws.com buster main" | tee /etc/apt/sources.list.d/ngrok.list && apt update && apt install ngrok


# Install Ollama
RUN curl https://ollama.ai/install.sh | sh

RUN pip install poetry

# Set the working directory
WORKDIR /app

# (Optional) Copy your Python project files and install dependencies
COPY ./defang-llama/pyproject.toml ./defang-llama/README.md ./defang-llama/poetry.lock ./defang-llama/poetry.toml /app/

RUN poetry install --no-dev

ENV MODEL="phi"
ENV FLASK_APP=defang_llama
ENV FLASK_ENV=production
ENV FLASK_RUN_HOST=0.0.0.0
ENV FLASK_RUN_PORT=5000

COPY ./defang-llama/ /app/

ENTRYPOINT [ "./run.sh" ]
