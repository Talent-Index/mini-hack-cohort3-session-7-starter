FROM python:3.12-slim

WORKDIR /app

# Install dependencies first, separately from copying the rest of the
# source, so Docker can cache this layer and skip reinstalling on every
# code change, only when requirements.txt actually changes.
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

# api_server.py listens on :8001 by default when run with uvicorn,
# exposed for the Session 7 "separate backend" pattern, docker-compose.yml
# maps this out for you already.
EXPOSE 8001

CMD ["sh", "-c", "echo 'Specify a script to run, e.g.: docker compose run python python rag.py \"your question\", or docker compose run -p 8001:8001 python uvicorn api_server:app --host 0.0.0.0 --port 8001'"]
