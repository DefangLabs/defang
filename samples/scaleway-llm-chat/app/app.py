import json
import logging
import os

import requests
from fastapi import FastAPI, Form
from fastapi.responses import HTMLResponse, JSONResponse, FileResponse
from fastapi.staticfiles import StaticFiles

app = FastAPI()
app.mount("/static", StaticFiles(directory="static"), name="static")

logging.basicConfig(level=logging.INFO)

LLM_URL = os.getenv("LLM_URL", "https://api.scaleway.ai/v1/") + "chat/completions"
MODEL_ID = os.getenv("LLM_MODEL", "llama-3.3-70b-instruct")


def get_api_key():
    return os.getenv("OPENAI_API_KEY", os.getenv("SCW_SECRET_KEY", ""))


@app.get("/", response_class=HTMLResponse)
async def home():
    return FileResponse("static/index.html", media_type="text/html")


@app.get("/health")
async def health():
    return {"status": "healthy"}


@app.post("/ask", response_class=JSONResponse)
async def ask(prompt: str = Form(...)):
    payload = {
        "model": MODEL_ID,
        "messages": [
            {"role": "system", "content": "You are a helpful assistant. Keep responses concise."},
            {"role": "user", "content": prompt},
        ],
        "stream": False,
    }

    reply = get_llm_response(payload)
    return {"prompt": prompt, "reply": reply}


def get_llm_response(payload):
    api_key = get_api_key()
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {api_key}",
    }

    logging.info(f"Sending request to {LLM_URL} with model {payload.get('model')}")

    try:
        response = requests.post(LLM_URL, headers=headers, data=json.dumps(payload), timeout=60)
    except requests.exceptions.RequestException as err:
        logging.error(f"Request failed: {err}")
        return f"Error: Could not reach LLM service."

    if response.status_code != 200:
        logging.error(f"LLM returned {response.status_code}: {response.text[:200]}")
        return f"Error: LLM returned status {response.status_code}"

    try:
        data = response.json()
        return data["choices"][0]["message"]["content"]
    except (KeyError, IndexError):
        return "Model returned an unexpected response."
