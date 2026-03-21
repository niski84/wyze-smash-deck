# ── Stage 1: Build ──────────────────────────────────────────────────────────
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o wyze-smash-deck ./cmd/wyzeferal/

# ── Stage 2: Runtime ─────────────────────────────────────────────────────────
# Scratch-like image — just ca-certs for HTTPS to Wyze API + tzdata for cron schedules
FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/wyze-smash-deck .
COPY web/ web/

# /app/data is mounted from the host so settings/devices/automations persist across restarts
VOLUME ["/app/data"]

EXPOSE 8082

ENV PORT=8082

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8082/api/health || exit 1

CMD ["./wyze-smash-deck"]
