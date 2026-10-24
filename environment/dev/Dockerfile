FROM golang:1.26.10-alpine

RUN apk add --no-cache gcc musl-dev tzdata \
    && adduser -D -u 1000 appuser
USER appuser
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download \
    && go install github.com/go-delve/delve/cmd/dlv@v1.26.3 \
    && go install github.com/air-verse/air@v1.67.4

COPY environment/dev/.air.toml /tmp/.air.toml
COPY . .
CMD ["air", "-c", "/tmp/.air.toml"]
