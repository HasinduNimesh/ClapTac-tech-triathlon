# Build context is the repository root (it shares pkg/agenttrace with the agents).
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY pkg ./pkg
COPY infrastructure/agent-manager ./infrastructure/agent-manager
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/agent-manager ./infrastructure/agent-manager

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && mkdir /data && chown nobody /data
COPY --from=build /out/agent-manager /usr/local/bin/agent-manager
ENV HTTP_ADDR=:8085 AGENT_TRACE_FILE=/data/traces.jsonl
EXPOSE 8085
USER nobody
ENTRYPOINT ["/usr/local/bin/agent-manager"]
