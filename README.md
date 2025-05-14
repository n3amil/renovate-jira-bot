# Renovate MR to Jira Issue bot

this is a simple go script to create a jira issue for every open Renovate MR. 
It checks MRs of a given User (in my case this is a group token in gitlab)

This meant to be run in a gitlab ci as a scheduled job

to build

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o renovate-jira-bot main.go
```
use the --dry-run option to check what would happen

Warning: this is "vibe coded" with chatgpt, I dont code in Go normally

