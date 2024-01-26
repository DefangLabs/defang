FROM nvidia/cuda:12.3.1-runtime-ubuntu22.04

# Install necessary build dependencies
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends software-properties-common curl build-essential git xxd openssh-server rsync && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir /var/run/sshd \
    && echo 'root:password' | chpasswd \
    && sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes\nPermitEmptyPasswords yes\nPasswordAuthentication yes/' /etc/ssh/sshd_config

# Install ngrok
RUN  curl -s https://ngrok-agent.s3.amazonaws.com/ngrok.asc | tee /etc/apt/trusted.gpg.d/ngrok.asc >/dev/null && echo "deb https://ngrok-agent.s3.amazonaws.com buster main" | tee /etc/apt/sources.list.d/ngrok.list && apt update && apt install ngrok

WORKDIR /app

COPY ./run.sh /app/run.sh

CMD [ "/app/run.sh" ]