# Dockerfile
# file: `Dockerfile`
FROM golang:1.25.1 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app .

FROM gcr.io/distroless/base-debian12
ENV PORT=8080
USER 65532:65532
COPY --from=builder /app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
