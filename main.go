package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type MergeRequest struct {
	IID         int    `json:"iid"`
	Title       string `json:"title"`
	Description string `json:"description"`
	WebURL      string `json:"web_url"`
	Author      struct {
		Username string `json:"username"`
	} `json:"author"`
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func getRenovateMRs() ([]MergeRequest, error) {
	projectID := getEnv("GITLAB_PROJECT_ID", os.Getenv("CI_PROJECT_ID"))
	gitlabURL := getEnv("GITLAB_URL", os.Getenv("CI_SERVER_URL"))
	token := getEnv("GITLAB_TOKEN", os.Getenv("CI_JOB_TOKEN"))
	renovateUsername := getEnv("RENOVATE_USERNAME", "renovate[bot]")
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests?state=opened", gitlabURL, projectID)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return nil, fmt.Errorf("Gitlab error: %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var mrs []MergeRequest
	if err := json.NewDecoder(resp.Body).Decode(&mrs); err != nil {
		return nil, err
	}

	var renovateMRs []MergeRequest
	for _, mr := range mrs {
		if mr.Author.Username == renovateUsername {
			renovateMRs = append(renovateMRs, mr)
		}
	}
	return renovateMRs, nil
}

func hasJiraKey(text string) (string, bool) {
	projectKey := os.Getenv("JIRA_PROJECT_KEY")
	jiraRegex := regexp.MustCompile(fmt.Sprintf(`%s-\d+`, regexp.QuoteMeta(projectKey)))
	match := jiraRegex.FindString(text)
	return match, match != ""
}

func containsKeyword(mr MergeRequest) bool {
	keywords := strings.Split(os.Getenv("KEYWORDS_TO_SKIP"), ",")
	for _, keyword := range keywords {
		if containsIgnoreCase(mr.Title, keyword) || containsIgnoreCase(mr.Description, keyword) {
			fmt.Printf("MR %d contains keywords to be skipped, skipping.\n", mr.IID)
			return true
		}
	}
	return false
}

func containsIgnoreCase(text, substr string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(substr))
}

func mrHasLinkedJira(mr MergeRequest, projectID, token, gitlabURL string) (bool, error) {
	// Check title and description first
	if _, found := hasJiraKey(mr.Title); found {
		return true, nil
	}
	if _, found := hasJiraKey(mr.Description); found {
		return true, nil
	}

	// Then check comments
	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes", gitlabURL, projectID, mr.IID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("PRIVATE-TOKEN", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var notes []struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&notes); err != nil {
		return false, err
	}

	for _, note := range notes {
		if _, found := hasJiraKey(note.Body); found {
			return true, nil
		}
	}
	return false, nil
}

func createJiraIssue(title, description string, dryRun bool) (string, error) {
	if dryRun {
		fmt.Printf("[DRY-RUN] Would create Jira issue:\n  Title: %s\n  Desc: %s\n\n", title, description)
		return "DRY-123", nil
	}

	jiraURL := os.Getenv("JIRA_URL")
	jiraUser := os.Getenv("JIRA_USER")
	jiraToken := os.Getenv("JIRA_API_TOKEN")
	projectKey := os.Getenv("JIRA_PROJECT_KEY")

	data := map[string]interface{}{
		"fields": map[string]interface{}{
			"project": map[string]string{
				"key": projectKey,
			},
			"summary":     title,
			"description": description,
			"issuetype": map[string]string{
				"name": "Task",
			},
		},
	}

	body, _ := json.Marshal(data)
	req, _ := http.NewRequest("POST", jiraURL+"/rest/api/2/issue", bytes.NewBuffer(body))
	req.SetBasicAuth(jiraUser, jiraToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Jira error: %s", string(respBody))
	}

	var respData struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return "", err
	}

	return respData.Key, nil
}

func commentOnMR(mrIID int, projectID, token, gitlabURL, jiraKey string, dryRun bool) error {
	comment := fmt.Sprintf("Jira issue created: [%s](%s/browse/%s)", jiraKey, os.Getenv("JIRA_URL"), jiraKey)
	if dryRun {
		fmt.Printf("[DRY-RUN] Would comment on MR %d: %s\n", mrIID, comment)
		return nil
	}

	url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests/%d/notes", gitlabURL, projectID, mrIID)
	payload := map[string]string{"body": comment}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitLab comment error: %s", string(respBody))
	}

	return nil
}

func main() {
	dryRunEnv := os.Getenv("DRY_RUN")
	dryRun, _ := strconv.ParseBool(dryRunEnv)
	if dryRunEnv == "" {
		dryRun = true
	}
	projectID := getEnv("GITLAB_PROJECT_ID", os.Getenv("CI_PROJECT_ID"))
	gitlabURL := getEnv("GITLAB_URL", os.Getenv("CI_SERVER_URL"))
	token := os.Getenv("GITLAB_TOKEN")
	mrs, err := getRenovateMRs()
	if err != nil {
		fmt.Println("Error fetching MRs:", err)
		os.Exit(1)
	}

	for _, mr := range mrs {

		if containsKeyword(mr) {
			continue
		}
		hasJira, err := mrHasLinkedJira(mr, projectID, token, gitlabURL)
		if err != nil {
			fmt.Printf("Error checking MR %d: %v\n", mr.IID, err)
			continue
		}
		if hasJira {
			fmt.Printf("MR %d already linked to a Jira issue, skipping.\n", mr.IID)
			continue
		}

		jiraKey, err := createJiraIssue(mr.Title, mr.WebURL, dryRun)
		if err != nil {
			fmt.Printf("Failed to create Jira issue for MR %d: %v\n", mr.IID, err)
			continue
		}

		err = commentOnMR(mr.IID, projectID, token, gitlabURL, jiraKey, dryRun)
		if err != nil {
			fmt.Printf("Failed to comment on MR %d: %v\n", mr.IID, err)
		}
	}
}
