# syntax=docker/dockerfile:1

# Build stage: compiles the bot. The SQLite driver is pure Go, so the
# binary is fully static and needs no C libraries at runtime.
FROM golang:1.27-alpine AS build
WORKDIR /src

# Download modules first, so this layer stays cached until go.mod changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bot ./cmd/bot

# The database directory must exist in the image with the runtime user as
# owner: a new Docker volume copies both from here.
RUN mkdir -p /out/data

# Runtime stage: just the binary, CA certificates for HTTPS to Telegram,
# and no shell. Runs as an unprivileged user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/bot /bot
COPY --from=build --chown=nonroot:nonroot /out/data /data

ENV DB_PATH=/data/queue.db
VOLUME /data
ENTRYPOINT ["/bot"]
