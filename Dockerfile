FROM golang@sha256:9349ed889adb906efa5ebc06485fe1b6a12fb265a01c9266a137bb1352565560 AS builder

WORKDIR /builder

ENV CGO_ENABLED=0

COPY go.mod go.sum /builder/
RUN go mod download

COPY *.go /builder/
RUN go build -v -o /dockpin

FROM scratch

COPY --from=builder /dockpin /bin/dockpin

ENTRYPOINT ["/bin/dockpin"]
