#!/bin/sh

echo "Starting ollama"
ollama serve &

# Wait for ollama to start
echo "Waiting for ollama to start"
i=1
while [ $i -le 20 ]
do
  curl -s -o /dev/null -w "%{http_code}" http://localhost:11434 | grep -q '200' && break
  sleep 1
  i=$((i+1))
done

if [ $i -eq 21 ]
then
  echo "Ollama did not start within 20 seconds"
  exit 1
fi
echo "Ollama started"

echo "Pulling model $MODEL"
ollama pull $MODEL

echo "Starting flask"
poetry run flask run