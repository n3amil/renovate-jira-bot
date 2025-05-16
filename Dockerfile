# Stage 1: build the binary
FROM golang:1.21 AS builder

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o renovate-jira-bot main.go

# Stage 2: minimal final image
FROM scratch

COPY --from=builder /src/renovate-jira-bot /renovate-jira-bot

ENTRYPOINT ["/renovate-jira-bot"]