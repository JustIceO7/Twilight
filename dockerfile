FROM golang:1.25.0-alpine3.22

WORKDIR /app
COPY . .

RUN go build -o twilight .

CMD ["./twilight"]