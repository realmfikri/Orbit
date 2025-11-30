# Build stage
FROM golang:1.22-bullseye AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY backend ./backend

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/orbitserver ./backend/cmd/orbitserver

# Final image
FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /out/orbitserver /app/orbitserver
EXPOSE 8080
ENTRYPOINT ["/app/orbitserver"]
