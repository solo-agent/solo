package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type taskFileEvidence struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Content     string `json:"content,omitempty"`
	URI         string `json:"uri,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

// OpenRoot keeps even symlink traversal inside the caller's working directory.
func localEvidenceFile(path string, write bool) (*os.File, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(cwd, absolute)
	if err != nil || !filepath.IsLocal(relative) {
		return nil, fmt.Errorf("evidence file must be inside the working directory")
	}
	root, err := os.OpenRoot(cwd)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if write {
		return root.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	}
	info, err := root.Lstat(relative)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("evidence must be a regular file, not a symlink or device")
	}
	return root.Open(relative)
}

func withTaskArtifact(body []byte, path, evidenceID string) ([]byte, error) {
	var request map[string]json.RawMessage
	if err := json.Unmarshal(body, &request); err != nil || request == nil {
		return nil, fmt.Errorf("--artifact requires a submission JSON object via --file")
	}
	file, err := localEvidenceFile(path, false)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 64001))
	if err != nil {
		return nil, err
	}
	if len(content) == 0 || len(content) > 64000 || !utf8.Valid(content) {
		return nil, fmt.Errorf("artifact must be a nonempty UTF-8 file of at most 64000 bytes")
	}
	var evidence []taskFileEvidence
	if raw := request["evidence"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &evidence); err != nil {
			return nil, err
		}
	}
	digest := sha256.Sum256(content)
	item := taskFileEvidence{ID: evidenceID, Description: "Actual file: " + filepath.Base(path), Content: string(content), SHA256: hex.EncodeToString(digest[:])}
	index := -1
	for i, existing := range evidence {
		if existing.ID != evidenceID {
			continue
		}
		if index >= 0 {
			return nil, fmt.Errorf("duplicate artifact evidence ID")
		}
		index = i
		if existing.Description != "" {
			item.Description = existing.Description
		}
	}
	if index < 0 {
		evidence = append(evidence, item)
	} else {
		evidence[index] = item
	}
	request["evidence"], _ = json.Marshal(evidence)
	request["artifact_version"], _ = json.Marshal(item.SHA256)
	return json.Marshal(request)
}

func exportTaskEvidence(body []byte, submissionID, evidenceID, output string) (string, error) {
	var submissions []struct {
		ID       string             `json:"id"`
		Evidence []taskFileEvidence `json:"evidence"`
	}
	if err := json.Unmarshal(body, &submissions); err != nil {
		return "", err
	}
	for _, submission := range submissions {
		if submission.ID != submissionID {
			continue
		}
		for _, item := range submission.Evidence {
			if item.ID != evidenceID {
				continue
			}
			if item.Content == "" {
				return "", fmt.Errorf("evidence has no inline content; a URI alone cannot be exported as verified source")
			}
			digest := sha256.Sum256([]byte(item.Content))
			actual := hex.EncodeToString(digest[:])
			if item.SHA256 != "" && !strings.EqualFold(item.SHA256, actual) {
				return "", fmt.Errorf("evidence content does not match its SHA256")
			}
			file, err := localEvidenceFile(output, true)
			if err != nil {
				return "", err
			}
			_, err = file.WriteString(item.Content)
			closeErr := file.Close()
			if err != nil {
				return "", err
			}
			if closeErr != nil {
				return "", closeErr
			}
			return actual, nil
		}
	}
	return "", fmt.Errorf("exact submission/evidence pair was not found")
}

func handleTaskEvidence(args []string, baseURL, token string) {
	fs := flag.NewFlagSet("task evidence", flag.ExitOnError)
	channel := fs.String("c", "", "Channel ID or #name")
	number := fs.Int("n", 0, "Task number")
	submission := fs.String("submission", "", "Exact immutable submission ID")
	evidence := fs.String("evidence", "", "Evidence ID")
	output := fs.String("output", "", "New file inside the current working directory")
	fs.Parse(args)
	if *channel == "" || *number <= 0 || *submission == "" || *evidence == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "solo: task evidence requires -c, -n, --submission, --evidence and --output")
		doExit(exitUsage)
		return
	}
	channelID, err := resolveChannelParam(baseURL, token, *channel)
	if err != nil {
		fmt.Fprintln(os.Stderr, "solo:", err)
		doExit(exitUsage)
		return
	}
	status, body, err := proxyRequest("task_lifecycle", channelID, "", "", token, *number, "submissions", "")
	if err != nil && allowDirectFallback() {
		status, body, err = doHTTP(http.MethodGet, fmt.Sprintf("%s/api/v1/channels/%s/tasks/%d/submissions", baseURL, channelID, *number), token, nil)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "solo:", err)
		doExit(exitUsage)
		return
	}
	if status >= 400 {
		handleNonProxyHTTPError(status, body)
		return
	}
	digest, err := exportTaskEvidence(body, *submission, *evidence, *output)
	if err != nil {
		fmt.Fprintln(os.Stderr, "solo:", err)
		doExit(exitBusiness)
		return
	}
	result, _ := json.Marshal(map[string]string{"submission_id": *submission, "evidence_id": *evidence, "path": *output, "sha256": digest})
	fmt.Println(string(result))
}
