#!/bin/sh

echo "@@ Starting SSH ..."
service ssh start

echo "@@ Configure ngrok ..."
ngrok config add-authtoken $NGROK_AUTH_TOKEN

echo "@@ Starting ngrok tcp 22 ..."
ngrok tcp 22 --log stdout