package staging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
)

// Compiler provenance is bounded syntax, separate from the producer's build pin.
// docs/adr/0149-compiler-provenance-and-security-patches.md:33.
var sourceCompilerPattern = regexp.MustCompile(`^go1\.(0|[1-9][0-9]{0,8})\.(0|[1-9][0-9]{0,8})$`)

// Requested identities and authenticated image bytes retain their authority.
// The compiler field cannot grant trust or select an installation generation.
// docs/adr/0149-compiler-provenance-and-security-patches.md:43.
func validateSourceIdentity(ctx context.Context, reader io.Reader, options Options) error {
	expected := map[string]string{"revision": options.Revision, "version": options.Version, "target": options.Target, "go_version": "", "kind": "source-build"}
	if options.Installation {
		expected["kind"] = "installation-source-build"
	}
	decoder := json.NewDecoder(&contextReader{ctx, reader})
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return errors.New("source metadata must be a JSON object")
	}
	seen := make(map[string]bool, len(expected))
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		required, known := expected[key]
		if !ok || !known || seen[key] {
			return fmt.Errorf("unknown or duplicate source field %q", token)
		}
		token, err = decoder.Token()
		if err != nil {
			return err
		}
		actual, ok := token.(string)
		if !ok {
			return fmt.Errorf("source field %q must be a string", key)
		}
		if key == "go_version" {
			if !sourceCompilerPattern.MatchString(actual) {
				return errors.New("source compiler provenance must be canonical stable Go syntax")
			}
		} else if actual != required {
			return fmt.Errorf("source field %q does not match requested identity", key)
		}
		seen[key] = true
	}
	if token, err = decoder.Token(); err != nil {
		return err
	}
	if token != json.Delim('}') || len(seen) != len(expected) {
		return errors.New("source metadata missing required fields")
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.Join(errors.New("source metadata has trailing data"), err)
	}
	return ctx.Err()
}
