package taskclient

import (
	"context"
	"net/http"
	"net/url"
)

type ImportRun struct {
	ID            string  `json:"id"`
	SourceSHA256  string  `json:"source_sha256"`
	ExpectedCount int     `json:"expected_count"`
	StagedCount   int     `json:"staged_count"`
	CommittedAt   *string `json:"committed_at"`
}

func (c *Client) BeginImport(ctx context.Context, hash string, count int) (ImportRun, error) {
	var out struct {
		Run ImportRun `json:"run"`
	}
	err := c.do(ctx, http.MethodPost, "/v1/imports", map[string]any{"operation_id": OperationID(), "source_sha256": hash, "expected_count": count}, &out)
	return out.Run, err
}
func (c *Client) StageImport(ctx context.Context, id string, tasks []Task) error {
	var out struct {
		Run ImportRun `json:"run"`
	}
	return c.do(ctx, http.MethodPost, "/v1/imports/"+url.PathEscape(id)+"/tasks", map[string]any{"tasks": tasks}, &out)
}
func (c *Client) CommitImport(ctx context.Context, id string) error {
	var out struct {
		RunID    string `json:"run_id"`
		Imported int    `json:"imported"`
	}
	return c.do(ctx, http.MethodPost, "/v1/imports/"+url.PathEscape(id)+"/commit", map[string]any{}, &out)
}
