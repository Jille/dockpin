FROM golang@sha256:0fa6504d3f1613f554c42131b8bf2dd1b2346fb69c2fc24a312e7cba6c87a71e AS builder

WORKDIR /builder

ENV CGO_ENABLED=0

COPY go.mod go.sum /builder/
RUN go mod download

COPY *.go /builder/
RUN go build -v -o /dockpin

FROM scratch

COPY --from=builder /dockpin /bin/dockpin

ENTRYPOINT ["/bin/dockpin"]
