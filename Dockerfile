# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

ARG TARGETOS TARGETARCH

COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=secret,id=gitlab_username,required=false \
    --mount=type=secret,id=gitlab_token,required=false \
    set -e; \
    if [ -f /run/secrets/gitlab_username ] && [ -f /run/secrets/gitlab_token ]; then \
        GITLAB_USERNAME="$(cat /run/secrets/gitlab_username)"; \
        GITLAB_TOKEN="$(cat /run/secrets/gitlab_token)"; \
        echo "Configuring GitLab private repo access"; \
        printf "machine gitlab.com\nlogin %s\npassword %s\n" \
          "$GITLAB_USERNAME" "$GITLAB_TOKEN" > ~/.netrc; \
        chmod 600 ~/.netrc; \
        git config --global url."https://${GITLAB_USERNAME}:${GITLAB_TOKEN}@gitlab.com/".insteadOf "https://gitlab.com/"; \
        go env -w GOPRIVATE=gitlab.com/lifegoeson-libs/* && \
        go env -w GONOPROXY=gitlab.com/lifegoeson-libs/* && \
        go env -w GONOSUMDB=gitlab.com/lifegoeson-libs/*; \
    else \
        echo "No GitLab credentials, skipping private repo setup"; \
    fi; \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
    -ldflags="-w -s" \
    -o main ./cmd/server

    
# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata wget

RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/main .
COPY --from=builder /app/config ./config
COPY --from=builder /app/.env* ./

RUN chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./main"]