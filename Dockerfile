FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY health-monitor/ /build/health-monitor/
WORKDIR /build/health-monitor
RUN go mod download
RUN CGO_ENABLED=0 go build -o /health-monitor ./cmd/module
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /health-monitor /
ENTRYPOINT ["/health-monitor"]
