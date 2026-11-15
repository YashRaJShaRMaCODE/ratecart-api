FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

FROM alpine:latest
WORKDIR /
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app
COPY --from=builder --chown=app:app /server /server
USER app
EXPOSE 8080
ENTRYPOINT ["/server"]
