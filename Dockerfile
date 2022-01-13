FROM golang@sha256:d36ec9839e6ebac63a53f3e15758ffa339b81dc5df6c9d41a18a3f9302bd0d90 AS builder

WORKDIR /builder

ENV CGO_ENABLED=0

COPY go.mod go.sum /builder/
RUN go mod download

COPY *.go /builder/
RUN go build -v -o /dockpin

FROM scratch

COPY --from=builder /dockpin /bin/dockpin

ENTRYPOINT ["/bin/dockpin"]
