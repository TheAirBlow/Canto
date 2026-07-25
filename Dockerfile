FROM golang:1.26-alpine AS build
WORKDIR /src

RUN apk add --no-cache git gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /out/canto ./cmd

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=build /out/canto ./canto

RUN mkdir -p /app/data
VOLUME ["/app/data"]

EXPOSE 8080
ENTRYPOINT ["./canto"]
