FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY pkg ./pkg
COPY tools/dev-oidc ./tools/dev-oidc
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/oidc ./tools/dev-oidc

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/oidc /usr/local/bin/oidc
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/oidc"]
