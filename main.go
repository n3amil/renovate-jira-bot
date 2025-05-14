package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

type MergeRequest struct {
	Title  string `json:"title"`
	WebURL string `json:"web_url"`
	Author struct {
		Username string `json:"username"`
	} `json:"author"`
}

func getRenovateMRs() ([]MergeRequest, error) {
	projectID := os.Getenv("GITLAB_PROJECT_ID")
	token := os.Getenv("GITLAB_TOKEN")

	url := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/merge_requests?state=opened", projectID)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("PRIVATE-TOKEN", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var mrs []MergeRequest
	if err := json.NewDecoder(resp.Body).Decode(&mrs); err != nil {
		return nil, err
	}

	// Filter for Renovate
	var renovateMRs []MergeRequest
	for _, mr := range mrs {
		if mr.Author.Username == "renovate[bot]" {
			renovateMRs = append(renovateMRs, mr)
		}
	}
	return renovateMRs, nil
}

func createJiraIssue(title, description string) error {
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
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("Jira error: %s", string(respBody))
	}

	return nil
}

func main() {
	mrs, err := getRenovateMRs()
	if err != nil {
		fmt.Println("Error fetching MRs:", err)
		os.Exit(1)
	}

	for _, mr := range mrs {
		err := createJiraIssue(mr.Title, mr.WebURL)
		if err != nil {
			fmt.Printf("Failed to create Jira issue for MR %s: %v\n", mr.WebURL, err)
		} else {
			fmt.Printf("Jira issue created for MR: %s\n", mr.WebURL)
		}
	}
}

