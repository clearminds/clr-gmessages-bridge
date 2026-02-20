FROM golang:1.24-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o gmessages-bridge .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates sqlite-libs
WORKDIR /app
COPY --from=builder /app/gmessages-bridge .
VOLUME /data
EXPOSE 7007
ENTRYPOINT ["./gmessages-bridge", "serve"]
