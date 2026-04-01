FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o maverick-agent .

FROM alpine:3.20
RUN apk add --no-cache curl
COPY --from=builder /app/maverick-agent /usr/local/bin/
ENTRYPOINT ["maverick-agent"]
