FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/crawlerx ./cmd/crawlerx
COPY internal ./internal
# timetzdata embeds the zone database, so TIMEZONE works on any base image.
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -tags timetzdata -ldflags="-s -w" -o /out/crawlerx ./cmd/crawlerx

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/crawlerx /crawlerx
COPY sources.demo.json /sources.demo.json
EXPOSE 8080
ENTRYPOINT ["/crawlerx"]
