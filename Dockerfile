# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Europe/Paris

WORKDIR /app

COPY --from=builder /server .
COPY frontend/ ./frontend/

EXPOSE 8080

CMD ["./server"]
