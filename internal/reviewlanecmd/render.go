package reviewlanecmd

import (
	"bytes"
	"context"
	"errors"
)

// RenderWorkflow shares the current enable projection without changing settings.
// docs/adr/0154-installed-review-workflow-migration.md:86.
func RenderWorkflow(ctx context.Context, data []byte, secret string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validSecret(secret) {
		return nil, errors.New("review lane: invalid repository secret name")
	}
	marker := []byte("__REVIEW_API_KEY_SECRET__")
	count := bytes.Count(data, marker)
	if count == 0 {
		return nil, errors.New("review lane: workflow template has no secret placeholder")
	}
	size := int64(len(managedHeader)+1+len(data)) + int64(count)*int64(len(secret)-len(marker))
	if size > fileLimit {
		return nil, errors.New("review lane: generated workflow exceeds 16 MiB")
	}
	return append([]byte(managedHeader+"\n"), bytes.ReplaceAll(data, marker, []byte(secret))...), ctx.Err()
}
