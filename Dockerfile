# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /crychic ./cmd/crychic

# distroless/static carries CA certificates (Telegram is HTTPS) and runs as
# uid 65532; the data volume must be writable by it.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /crychic /crychic
ENV CRYCHIC_DATA_DIR=/data
VOLUME /data
ENTRYPOINT ["/crychic"]
