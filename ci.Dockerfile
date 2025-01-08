# build app
FROM --platform=$BUILDPLATFORM golang:1.23-alpine3.20 AS app-builder
RUN apk add --no-cache git tzdata

ENV SERVICE=ptparchiver

WORKDIR /src

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

COPY . ./

ARG VERSION=dev
ARG REVISION
ARG BUILDTIME
ARG BUILDER
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

RUN REVISION=$(git rev-parse --short HEAD 2>/dev/null || echo "none") && \
    BUILDTIME=$(date -u +'%Y-%m-%dT%H:%M:%SZ') && \
    BUILDER=${BUILDER:-docker} && \
    export GOOS=$TARGETOS; \
    export GOARCH=$TARGETARCH; \
    [[ "$GOARCH" == "amd64" ]] && export GOAMD64=$TARGETVARIANT; \
    [[ "$GOARCH" == "arm" ]] && [[ "$TARGETVARIANT" == "v6" ]] && export GOARM=6; \
    [[ "$GOARCH" == "arm" ]] && [[ "$TARGETVARIANT" == "v7" ]] && export GOARM=7; \
    echo $GOARCH $GOOS $GOARM$GOAMD64; \
    go build -ldflags "-s -w \
    -X github.com/s0up4200/ptparchiver-go/pkg/version.Version=${VERSION} \
    -X github.com/s0up4200/ptparchiver-go/pkg/version.Commit=${REVISION} \
    -X github.com/s0up4200/ptparchiver-go/pkg/version.Date=${BUILDTIME} \
    -X github.com/s0up4200/ptparchiver-go/pkg/version.BuiltBy=${BUILDER}" \
    -o /out/bin/ptparchiver cmd/ptparchiver/main.go

# build runner
FROM alpine:latest AS runner

LABEL org.opencontainers.image.source="https://github.com/s0up4200/ptparchiver-go"
LABEL org.opencontainers.image.licenses="GPL-2.0-or-later"
LABEL org.opencontainers.image.base.name="alpine:latest"

ENV HOME="/config" \
    XDG_CONFIG_HOME="/config" \
    XDG_DATA_HOME="/config"

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
VOLUME /config

COPY --link --from=app-builder /out/bin/ptparchiver /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/ptparchiver"]