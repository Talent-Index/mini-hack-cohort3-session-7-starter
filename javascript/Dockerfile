FROM node:20-slim

WORKDIR /app

# Install dependencies first, separately from copying the rest of the
# source, so Docker can cache this layer and skip reinstalling on every
# code change, only when package.json actually changes.
COPY package.json package-lock.json* ./
RUN npm install

COPY . .

CMD ["sh", "-c", "echo 'Specify a script to run, e.g.: docker compose run javascript npm run rag -- \"your question\"'"]
