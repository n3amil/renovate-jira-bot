export $(grep -v '^#' .env | xargs)
./renovate-jira-bot

