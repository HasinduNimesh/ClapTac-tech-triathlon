# Build from repository root:
# docker build -f services/order-service/Dockerfile --build-arg SERVICE=order-service .
ARG SERVICE=order-service
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
