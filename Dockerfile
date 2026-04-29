# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o rawth ./cmd/rawth/

# Runtime stage — as minimal as it gets
FROM alpine:3.19

RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/rawth .

EXPOSE 8080 6379

# Use $PORT if set by the cloud platform (Railway, Render, Fly.io), otherwise default to 8080
CMD sh -c "./rawth serve --http ${PORT:-8080} --tcp 6379 --data /app/rawth.db"
