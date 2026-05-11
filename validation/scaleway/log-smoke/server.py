from http.server import BaseHTTPRequestHandler, HTTPServer
from datetime import datetime, timezone
import sys
import threading
import time


def stamp() -> str:
    return datetime.now(timezone.utc).isoformat()


def log(message: str) -> None:
    print(f"defang-log-smoke {stamp()} {message}", flush=True)


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        log(f"request path={self.path}")
        body = b"defang log smoke ok\n"
        self.send_response(200)
        self.send_header("content-type", "text/plain")
        self.send_header("content-length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):
        sys.stderr.write(f"defang-log-smoke-access {stamp()} {format % args}\n")
        sys.stderr.flush()


def heartbeat() -> None:
    while True:
        log("heartbeat")
        time.sleep(5)


if __name__ == "__main__":
    log("starting server port=8080")
    threading.Thread(target=heartbeat, daemon=True).start()
    HTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
