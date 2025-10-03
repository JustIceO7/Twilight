FROM golang:1.25.0-alpine3.22

WORKDIR /app

COPY go.mod go.sum ./

RUN apk add --no-cache ffmpeg ca-certificates opus-dev build-base yt-dlp \
    && go mod download

COPY . .

RUN mkdir -p /app/cache && chmod 755 /app/cache \
    && go build -o twilight .

CMD ["./twilight"]
