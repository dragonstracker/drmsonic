FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY ui ./ui

ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags '-s -w' -o /out/drmsonic ./cmd/drmsonic

FROM alpine:3.20

RUN adduser -D -H -u 10001 drmsonic

WORKDIR /app
COPY --from=build /out/drmsonic /app/drmsonic
COPY providers/example.json /app/providers/example.json

RUN mkdir -p /app/providers /app/data && chown -R drmsonic:drmsonic /app

USER drmsonic

EXPOSE 7779
VOLUME ["/app/providers", "/app/data"]

ENTRYPOINT ["/app/drmsonic"]
CMD ["-p", "7779", "-b", "0.0.0.0", "-providers", "providers", "-c", "data/drmsonic.json"]
