FROM golang:1.23 AS builder

ARG VERSION=dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /fcm ./cmd/fcm

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /fcm /usr/local/bin/fcm

ENTRYPOINT ["fcm"]
