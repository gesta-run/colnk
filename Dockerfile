FROM golang:1.24-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY integration ./integration
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/colnk-server ./cmd/colnk-server

FROM alpine:3.24 AS server
RUN apk add --no-cache ca-certificates fuse3 iproute2 iptables curl bind-tools
RUN mkdir -p /mnt/local && adduser -D -u 1000 agent
RUN echo user_allow_other > /etc/fuse.conf
COPY --from=build /out/colnk-server /usr/local/bin/colnk-server
ENTRYPOINT ["/usr/local/bin/colnk-server"]
