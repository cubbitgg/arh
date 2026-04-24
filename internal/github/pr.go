package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// PRData holds all information fetched from GitHub for a pull request.
type PRData struct {
	Owner        string
	Repo         string
	Number       int
	Title        string
	Body         string
	BranchName   string
	BaseSHA      string
	HeadSHA      string
	Labels       []string
	Commits      []CommitMsg
	ChangedFiles []ChangedFile
	Diff         string
	PerFileDiff  map[string]string
}

// CommitMsg is a commit's subject and full message.
type CommitMsg struct {
	SHA     string
	Subject string
	Body    string
}

// ChangedFile is metadata for a file changed in the PR.
type ChangedFile struct {
	Path      string
	Status    string
	Additions int
	Deletions int
}

type prRef struct {
	owner  string
	repo   string
	number int
}

// parsePRRef parses "owner/repo#N" or a full GitHub PR URL.
func parsePRRef(s string) (prRef, error) {
	// Full URL: https://github.com/owner/repo/pull/N
	urlRe := regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/pull/(\d+)`)
	if m := urlRe.FindStringSubmatch(s); len(m) == 4 {
		n, _ := strconv.Atoi(m[3])
		return prRef{owner: m[1], repo: m[2], number: n}, nil
	}
	// Short form: owner/repo#N
	shortRe := regexp.MustCompile(`^([^/]+)/([^#]+)#(\d+)$`)
	if m := shortRe.FindStringSubmatch(s); len(m) == 4 {
		n, _ := strconv.Atoi(m[3])
		return prRef{owner: m[1], repo: m[2], number: n}, nil
	}
	return prRef{}, fmt.Errorf("unrecognized PR reference %q; use owner/repo#N or a GitHub PR URL", s)
}

func runGH(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if ok := false; !ok {
			_ = exitErr
		}
		return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

type ghPRResponse struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Head   struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		SHA string `json:"sha"`
	} `json:"base"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

type ghCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
	} `json:"commit"`
}

type ghFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// FetchPR fetches all PR data from GitHub via the gh CLI.
func FetchPR(ctx context.Context, ref string) (*PRData, error) {
	r, err := parsePRRef(ref)
	if err != nil {
		return nil, err
	}

	// Fetch PR metadata
	metaBytes, err := runGH(ctx, "api", fmt.Sprintf("repos/%s/%s/pulls/%d", r.owner, r.repo, r.number))
	if err != nil {
		return nil, fmt.Errorf("fetching PR metadata: %w", err)
	}
	var meta ghPRResponse
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return nil, fmt.Errorf("parsing PR metadata: %w", err)
	}

	// Fetch commits
	commitsBytes, err := runGH(ctx, "api", fmt.Sprintf("repos/%s/%s/pulls/%d/commits", r.owner, r.repo, r.number))
	if err != nil {
		return nil, fmt.Errorf("fetching commits: %w", err)
	}
	var rawCommits []ghCommit
	if err := json.Unmarshal(commitsBytes, &rawCommits); err != nil {
		return nil, fmt.Errorf("parsing commits: %w", err)
	}
	commits := make([]CommitMsg, 0, len(rawCommits))
	for _, c := range rawCommits {
		parts := strings.SplitN(c.Commit.Message, "\n", 2)
		subject := parts[0]
		body := ""
		if len(parts) > 1 {
			body = strings.TrimSpace(parts[1])
		}
		commits = append(commits, CommitMsg{SHA: c.SHA, Subject: subject, Body: body})
	}

	// Fetch changed files
	filesBytes, err := runGH(ctx, "api", fmt.Sprintf("repos/%s/%s/pulls/%d/files", r.owner, r.repo, r.number))
	if err != nil {
		return nil, fmt.Errorf("fetching changed files: %w", err)
	}
	var rawFiles []ghFile
	if err := json.Unmarshal(filesBytes, &rawFiles); err != nil {
		return nil, fmt.Errorf("parsing changed files: %w", err)
	}
	changedFiles := make([]ChangedFile, 0, len(rawFiles))
	for _, f := range rawFiles {
		changedFiles = append(changedFiles, ChangedFile{
			Path:      f.Filename,
			Status:    f.Status,
			Additions: f.Additions,
			Deletions: f.Deletions,
		})
	}

	// Fetch diff
	diffBytes, err := runGH(ctx, "pr", "diff", strconv.Itoa(r.number), "--repo", fmt.Sprintf("%s/%s", r.owner, r.repo))
	if err != nil {
		return nil, fmt.Errorf("fetching diff: %w", err)
	}
	diff := string(diffBytes)

	// Extract labels
	labels := make([]string, 0, len(meta.Labels))
	for _, l := range meta.Labels {
		labels = append(labels, l.Name)
	}

	return &PRData{
		Owner:        r.owner,
		Repo:         r.repo,
		Number:       meta.Number,
		Title:        meta.Title,
		Body:         meta.Body,
		BranchName:   meta.Head.Ref,
		BaseSHA:      meta.Base.SHA,
		HeadSHA:      meta.Head.SHA,
		Labels:       labels,
		Commits:      commits,
		ChangedFiles: changedFiles,
		Diff:         diff,
		PerFileDiff:  splitDiffByFile(diff),
	}, nil
}

// splitDiffByFile splits a unified diff into per-file sections.
func splitDiffByFile(diff string) map[string]string {
	perFile := make(map[string]string)
	// Split on "diff --git" header lines
	sections := regexp.MustCompile(`(?m)^diff --git `).Split(diff, -1)
	for _, section := range sections {
		if section == "" {
			continue
		}
		full := "diff --git " + section
		// Extract the b/ path from the first line: "diff --git a/foo b/foo"
		firstLine := strings.SplitN(section, "\n", 2)[0]
		fields := strings.Fields(firstLine)
		if len(fields) >= 2 {
			path := strings.TrimPrefix(fields[1], "b/")
			perFile[path] = full
		}
	}
	return perFile
}
