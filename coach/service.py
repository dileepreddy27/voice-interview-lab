"""Stateless local coaching HTTP service. No model credentials required."""
import json
import math
import os
import re
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def evaluate(text, duration):
    if not isinstance(text, str) or len(text) > 24000:
        raise ValueError("text must be a string of at most 24000 characters")
    if isinstance(duration, bool) or not isinstance(duration, (float, int)):
        raise ValueError("duration_seconds must be a number")
    if not math.isfinite(duration) or not 0 < duration <= 120:
        raise ValueError("duration_seconds must be in (0, 120]")
    words = re.findall(r"\b[\w']+\b", text.lower())
    patterns = {
        "Situation": r"\b(project|team|customer|system|service|when)\b",
        "Task": r"\b(needed|goal|responsible|task|had to|wanted)\b",
        "Action": r"\b(i built|i implemented|i designed|i tested|i added|i led|i measured)\b",
        "Result": r"\b(result|reduced|improved|increased|saved|delivered)\b",
    }
    signals = {name: bool(re.search(pattern, text, re.I)) for name, pattern in patterns.items()}
    missing = [name.lower() for name, found in signals.items() if not found]
    fillers = sum(word in {"um", "uh", "erm"} for word in words)
    quantified = bool(re.search(r"\b\d+(?:\.\d+)?(?:%|\b)", text))
    tips = []
    if not words:
        tips.append("No finalized speech yet. Try a short example from your own experience.")
    elif missing:
        tips.append("Make your " + ", ".join(missing) + " explicit with one concrete sentence.")
    if not quantified:
        tips.append("If you have a measured outcome, include it; never invent a number.")
    if fillers:
        tips.append("Try a brief pause in place of filler words.")
    if not tips:
        tips.append("Practice explaining the tradeoff and how you verified the result.")
    return {
        "engine": "local-heuristic-v1", "word_count": len(words),
        "words_per_minute": round(len(words) * 60 / duration, 1),
        "filler_count": fillers, "structure_signals": signals,
        "quantified_detail": quantified, "tips": tips,
        "disclaimer": "Keyword signals are practice cues, not a hiring score or semantic assessment.",
    }


def record_metrics(result):
    """Optional Redis: anonymous aggregate metrics only, 1-hour expiry."""
    url = os.getenv("REDIS_URL")
    if not url:
        return "disabled"
    try:
        import redis
        client = redis.Redis.from_url(url, socket_connect_timeout=0.5, socket_timeout=0.5)
        payload = {key: result[key] for key in ("word_count", "words_per_minute", "filler_count")}
        client.set("voice:metrics:" + uuid.uuid4().hex, json.dumps(payload), ex=3600)
        client.close()
        return "stored-anonymous-metrics"
    except Exception:
        # Never return connection strings, credentials, or provider error bodies.
        return "unavailable"


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass  # No transcripts, requests, or identifiers in access logs.

    def respond(self, status, payload):
        data = json.dumps(payload, allow_nan=False).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        self.respond(200 if self.path == "/healthz" else 404, {"service": "coach"})

    def do_POST(self):
        if self.path != "/evaluate":
            return self.respond(404, {"error": "not found"})
        try:
            size = int(self.headers.get("Content-Length", "0"))
            if not 0 < size <= 100000:
                raise ValueError("invalid body size")
            self.connection.settimeout(5)
            body = json.loads(self.rfile.read(size))
            result = evaluate(body["text"], body["duration_seconds"])
            result["storage"] = record_metrics(result)
            self.respond(200, result)
        except (ValueError, KeyError, TypeError, TimeoutError):
            self.respond(400, {"error": "invalid evaluation request"})


if __name__ == "__main__":
    address = os.getenv("COACH_BIND", "127.0.0.1")
    server = ThreadingHTTPServer((address, int(os.getenv("COACH_PORT", "8091"))), Handler)
    print("Coaching service ready", flush=True)
    server.serve_forever()
