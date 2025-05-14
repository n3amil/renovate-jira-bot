export $(grep -v '^#' .env | xargs)
./renovate-jira-bot --dry-run

