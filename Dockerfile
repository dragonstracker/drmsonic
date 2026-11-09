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

RUN mkdir -p /app/providers && chown -R drmsonic:drmsonic /app

USER drmsonic

EXPOSE 1234
VOLUME ["/app/providers"]

ENTRYPOINT ["/app/drmsonic"]
CMD ["-p", "1234", "-b", "0.0.0.0", "-providers", "providers"]
