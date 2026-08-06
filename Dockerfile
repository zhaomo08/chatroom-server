FROM golang:1.26 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /chatroom-server ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /chatroom-server /chatroom-server
EXPOSE 8080
ENTRYPOINT ["/chatroom-server"]
