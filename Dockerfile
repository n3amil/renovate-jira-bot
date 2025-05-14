FROM golang:1.21-alpine

WORKDIR /app
COPY . .

RUN go build -o renovate-jira-bot .

CMD ["./renovate-jira-bot"]

