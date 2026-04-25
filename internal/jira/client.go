package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// JiraClient fetches Jira issue data.
type JiraClient interface {
	FetchIssue(ctx context.Context, issueKey string) (*Issue, error)
}

// Issue holds the relevant fields fetched from a Jira issue.
type Issue struct {
	Key                string
	Summary            string
	Description        string
	AcceptanceCriteria string
	Status             string
	IssueType          string
}

// Client is a concrete JiraClient using Jira REST API v3.
type Client struct {
	baseURL    string
	email      string
	apiToken   string
	httpClient *http.Client
}

// NewClient creates a Jira REST API v3 client.
func NewClient(baseURL, email, apiToken string) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("Jira base_url is required")
	}
	if email == "" {
		return nil, fmt.Errorf("Jira user email is required (check JIRA_USER_EMAIL env var)")
	}
	if apiToken == "" {
		return nil, fmt.Errorf("Jira API token is required (check JIRA_API_TOKEN env var)")
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		email:      email,
		apiToken:   apiToken,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// FetchIssue retrieves a Jira issue by its key (e.g. "PROJ-123").
func (c *Client) FetchIssue(ctx context.Context, issueKey string) (*Issue, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s?fields=summary,description,status,issuetype,customfield_10016", c.baseURL, issueKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Jira request for %s: %w", issueKey, err)
	}
	req.Header.Set("Authorization", "Basic "+basicAuth(c.email, c.apiToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching Jira issue %s: %w", issueKey, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading Jira response for %s: %w", issueKey, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching Jira issue %s: HTTP %d: %s", issueKey, resp.StatusCode, string(body))
	}

	return parseIssueResponse(issueKey, body)
}

func parseIssueResponse(issueKey string, body []byte) (*Issue, error) {
	var raw struct {
		Fields struct {
			Summary   string                `json:"summary"`
			Status    struct{ Name string } `json:"status"`
			IssueType struct{ Name string } `json:"issuetype"`
			// Description is Atlassian Document Format (ADF) — nested JSON
			Description json.RawMessage `json:"description"`
			// Acceptance criteria custom field (common Jira Cloud field)
			AcceptanceCriteria json.RawMessage `json:"customfield_10016"`
		} `json:"fields"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing Jira response for %s: %w", issueKey, err)
	}

	description := extractTextFromADF(raw.Fields.Description)

	// Try custom field for acceptance criteria first, then scan description text.
	ac := extractTextFromADF(raw.Fields.AcceptanceCriteria)
	if ac == "" {
		ac = extractACFromText(description)
	}

	return &Issue{
		Key:                issueKey,
		Summary:            raw.Fields.Summary,
		Description:        description,
		AcceptanceCriteria: ac,
		Status:             raw.Fields.Status.Name,
		IssueType:          raw.Fields.IssueType.Name,
	}, nil
}

// extractTextFromADF recursively walks an Atlassian Document Format node and
// concatenates all text content into a plain string.
func extractTextFromADF(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	var node struct {
		Type    string            `json:"type"`
		Text    string            `json:"text"`
		Content []json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &node); err != nil {
		return ""
	}

	if node.Type == "text" {
		return node.Text
	}
	if node.Type == "hardBreak" {
		return "\n"
	}

	var parts []string
	for _, child := range node.Content {
		if text := extractTextFromADF(child); text != "" {
			parts = append(parts, text)
		}
	}

	sep := ""
	if node.Type == "paragraph" || node.Type == "heading" || node.Type == "listItem" {
		sep = "\n"
	}
	return strings.Join(parts, "") + sep
}

// extractACFromText scans plain text for an "Acceptance Criteria" section heading
// and returns the text that follows it.
func extractACFromText(text string) string {
	markers := []string{"Acceptance Criteria", "acceptance criteria", "AC:", "Acceptance criteria:"}
	for _, marker := range markers {
		_, after, found := strings.Cut(text, marker)
		if !found {
			continue
		}
		after = strings.TrimSpace(after)
		if after != "" {
			return after
		}
	}
	return ""
}

func basicAuth(email, token string) string {
	return base64.StdEncoding.EncodeToString([]byte(email + ":" + token))
}
