FROM ubuntu:latest

RUN apt-get update && apt-get install -y direnv make

RUN echo 'eval "$(direnv hook bash)"' >> ~/.bashrc
