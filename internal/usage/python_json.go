package usage

import (
	"context"
	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

type jsonSyntaxError = jsonvalue.SyntaxError

// Shared decoding retains the permissive native-stream contract.
// docs/adr/0069-go-budget-ledger-admission.md:179.
func decodePythonJSON(ctx context.Context, data []byte) (any, error) {
	return jsonvalue.Decode(ctx, data)
}
