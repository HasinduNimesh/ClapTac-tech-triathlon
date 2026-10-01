# Build from repository root:
# docker build -f services/agent-orchestrator/Dockerfile --build-arg SERVICE=agent-orchestrator .
ARG SERVICE=agent-orchestrator
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY pkg ./pkg
COPY services ./services
ARG SERVICE
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./services/${SERVICE}/cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/server /usr/local/bin/server
EXPOSE 8080
USER nobody
ENTRYPOINT ["/usr/local/bin/server"]
