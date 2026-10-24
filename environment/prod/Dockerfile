FROM golang:1.26.10-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -trimpath -o /out/http-api-template ./cmd/api


FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /out/http-api-template .
CMD ["/app/http-api-template"]
