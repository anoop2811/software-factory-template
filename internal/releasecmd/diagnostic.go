package releasecmd

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/anoop2811/software-factory-template/internal/output"
)

// Diagnostic publication has an independent bounded lifetime, including cleanup
// notices after cancellation. Raw inherited streams keep their original type.
// docs/adr/0122-default-packaged-go-upgrade.md:84.
func writeDiagnostic(stderr io.Writer, format string, args ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return output.WriteEvent(ctx, stderr, []byte(fmt.Sprintf(format, args...)))
}
