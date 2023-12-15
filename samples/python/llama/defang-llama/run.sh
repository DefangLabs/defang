#!/bin/sh

echo "Model file"
echo $MODEL_FILE

echo "Model url"
echo $MODEL_URL

if [ ! -f "/app/models/${MODEL_FILE}" ]; then
    curl -L $MODEL_URL -o "/app/models/${MODEL_FILE}"
fi

poetry run flask run