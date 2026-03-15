# ─── Stage 1: Build ───
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /testgen-agent ./cmd/agent/

# Устанавливаем goimports
RUN go install golang.org/x/tools/cmd/goimports@latest

# ─── Stage 2: Runtime ───
FROM golang:1.23-alpine

RUN apk add --no-cache git

# Копируем бинарник агента
COPY --from=builder /testgen-agent /usr/local/bin/testgen-agent

# Копируем goimports
COPY --from=builder /root/go/bin/goimports /usr/local/bin/goimports

ENTRYPOINT ["testgen-agent"]
