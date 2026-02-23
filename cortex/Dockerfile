FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /cortex ./cmd/


FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /cortex /cortex

EXPOSE 8080

ENV GITHUB_ORG=KoalbyMQP
ENV GITHUB_REPOS=Core
ENV DOCKERHUB_ORG=koalby

ENTRYPOINT ["/cortex"]
