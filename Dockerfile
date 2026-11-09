FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" \
    -o /out/breezewiki-go ./cmd/breezewiki

FROM alpine:3.23
RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /out/breezewiki-go /usr/local/bin/breezewiki-go
ENV BW_BIND_HOST=0.0.0.0
EXPOSE 10416
ENTRYPOINT ["/usr/local/bin/breezewiki-go"]
