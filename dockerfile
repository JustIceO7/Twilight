FROM golang:1.25.0-alpine3.22

WORKDIR /app

COPY go.mod go.sum ./

RUN apk add --no-cache ffmpeg git ca-certificates opus-dev build-base
RUN go mod download

COPY . .

RUN mkdir -p /app/cache && chmod 755 /app/cache
RUN go build -o twilight .

CMD ["./twilight"]
