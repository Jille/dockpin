FROM golang@sha256:6fbd2d3398db924f8d708cf6e94bd3a436bb468195daa6a96e80504e0a9615f2 AS builder

WORKDIR /builder

ENV CGO_ENABLED=0

COPY go.mod go.sum /builder/
RUN go mod download

COPY *.go /builder/
RUN go build -v -o /dockpin

FROM scratch

COPY --from=builder /dockpin /bin/dockpin

ENTRYPOINT ["/bin/dockpin"]
