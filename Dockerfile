ARG GO_VERSION=1.26
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/sample-scorer ./cmd/sample-scorer

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/sample-scorer /sample-scorer

EXPOSE 8000

ENTRYPOINT ["/sample-scorer"]
