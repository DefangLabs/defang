#!/bin/sh

echo "Model file"
echo $MODEL_FILE

echo "Model url"
echo $MODEL_URL

curl -L $MODEL_URL -o "/app/models/${MODEL_FILE}"

poetry run flask run