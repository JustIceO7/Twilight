FROM golang:1.25.0-alpine3.22

WORKDIR /app

COPY . .

CMD ["go", "run", "main.go"]