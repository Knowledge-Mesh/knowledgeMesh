FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY web/ web/
RUN CGO_ENABLED=0 go build -o km-fabric .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/km-fabric .
RUN mkdir -p /data
ENV KM_LEDGER_PATH=/data/ledger.txt
ENV PORT=8080
EXPOSE 8080
CMD ["./km-fabric"]
