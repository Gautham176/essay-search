# Multi-stage build: produce a small final image with just the binary
# and the embedded HTML (no Go toolchain in the runtime image).

# --- Stage 1: build ---
FROM golang:1.26-alpine AS build

WORKDIR /src

# Cache dependency download as a separate layer: rebuilds skip this
# unless go.mod or go.sum changes.
COPY go.mod go.sum ./
RUN go mod download

# Now copy the rest and build. CGO disabled so the binary is fully static
# and works on a `scratch`-based runtime image. Stripping symbols
# (-ldflags="-s -w") shrinks the binary by ~30%.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /out/serve \
    ./cmd/serve

# --- Stage 2: runtime ---
# Use a minimal image. `alpine` gives us a shell for debugging; `scratch`
# is smaller but harder to introspect when something breaks. Alpine wins
# the tradeoff for now.
FROM alpine:3.20

# Non-root user, in case anyone ever runs this with elevated container
# privileges. Defense in depth.
RUN adduser -D -u 10001 app
USER app

WORKDIR /app
COPY --from=build /out/serve /app/serve

# The server reads $DATABASE_URL for the DB connection and listens on
# whatever -addr it's given. Default both via the CMD.
EXPOSE 8080
ENV PORT=8080

ENTRYPOINT ["/app/serve"]
CMD ["-addr=:8080"]