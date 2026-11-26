package contextcmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"strings"
)

// specs/003-task-context-and-skills.md:304 defines the two canonical objects.
type selection struct {
	File string `json:"file"`
}

type request struct {
	Version  int         `json:"version"`
	Catalogs []string    `json:"catalogs"`
	Select   []selection `json:"select"`
	MaxBytes int         `json:"max_bytes"`
}

type citation struct {
	Path   string  `json:"path"`
	Lines  *[2]int `json:"lines"`
	SHA256 string  `json:"sha256"`
}

type item struct {
	Kind           string     `json:"kind"`
	Representation string     `json:"representation"`
	Citations      []citation `json:"citations"`
	Text           string     `json:"text"`
}

type payload struct {
	Items []item `json:"items"`
}

type included struct {
	Citations []citation `json:"citations"`
	Reason    string     `json:"reason"`
	ItemIndex int        `json:"item_index"`
}

type preview struct {
	Version      int        `json:"version"`
	Digest       string     `json:"digest"`
	Payload      payload    `json:"payload"`
	PayloadBytes int        `json:"payload_bytes"`
	Included     []included `json:"included"`
	Omitted      []any      `json:"omitted"`
}

func canonical(value any) (jsontext.Value, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	jcs := jsontext.Value(raw)
	if err := jcs.Canonicalize(); err != nil {
		return nil, err
	}
	return jcs, nil
}

func previewFiles(ctx context.Context, files []string, maxBytes int) (preview, error) {
	if len(files) == 0 || len(files) > selectionLimit {
		return preview{}, invalid("select between 1 and %d files with --file", selectionLimit)
	}
	if maxBytes < 1 || maxBytes > maxPayloadBytes {
		return preview{}, invalid("--max-bytes must be between 1 and %d", maxPayloadBytes)
	}
	req := request{Version: 1, Catalogs: []string{}, MaxBytes: maxBytes}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		path, err := normalizePath(file)
		if err != nil {
			return preview{}, err
		}
		paths = append(paths, path)
		req.Select = append(req.Select, selection{File: path})
	}
	sources, err := readSources(ctx, paths)
	if err != nil {
		return preview{}, err
	}
	p := preview{Version: 1, Omitted: []any{}}
	for i, source := range sources {
		c := citation{Path: source.path, SHA256: fmt.Sprintf("%x", sha256.Sum256(source.data))}
		if len(source.data) != 0 {
			n := bytes.Count(source.data, []byte{'\n'})
			if source.data[len(source.data)-1] != '\n' {
				n++
			}
			c.Lines = &[2]int{1, n}
		}
		citations := []citation{c}
		p.Payload.Items = append(p.Payload.Items, item{"source", "original", citations, string(source.data)})
		p.Included = append(p.Included, included{citations, "explicit", i})
	}
	body, err := canonical(p.Payload)
	if err != nil {
		return preview{}, err
	}
	p.PayloadBytes = len(body)
	if len(body) > maxBytes {
		var sizes strings.Builder
		for _, source := range sources {
			fmt.Fprintf(&sizes, "; %s (%d source bytes)", safeName(source.path), len(source.data))
			if sizes.Len() >= diagnosticLimit {
				break
			}
		}
		return preview{}, invalid("required payload needs %d bytes, limit %d%s", len(body), maxBytes, sizes.String())
	}
	envelope, err := canonical(struct {
		Version      int             `json:"version"`
		Request      request         `json:"request"`
		Declarations []any           `json:"declarations"`
		Payload      json.RawMessage `json:"payload"`
	}{1, req, []any{}, json.RawMessage(body)})
	if err != nil {
		return preview{}, err
	}
	p.Digest = fmt.Sprintf("%x", sha256.Sum256(envelope))
	return p, nil
}
