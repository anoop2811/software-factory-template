package loop

import (
	"context"
	"io"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

func budgetRequest(r Request, role string) budget.Request {
	return budget.Request{Session: r.Session, Task: r.Task, Harness: r.Harness, Role: role, Model: r.Environment["FACTORY_LOOP_"+strings.ToUpper(r.Harness)+"_"+strings.ToUpper(role)+"_MODEL"]}
}

// Normalize only after bounded regular-file reads; freshness reuses the same loader.
// docs/adr/0076-go-bounded-loop-controller.md:55.
func loadLoopPrompt(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if path == "" {
		return "", manualError()
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return "", manualError()
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 2<<20 {
		return "", manualError()
	}
	data, err := io.ReadAll(io.LimitReader(boundedContextReader{ctx, file}, (2<<20)+1))
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err != nil || len(data) > 2<<20 || !utf8.Valid(data) {
		return "", manualError()
	}
	text := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	if len(text) > 1<<20 {
		return "", manualError()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return text, nil
}

type reviewVerdict struct {
	Verdict  string
	Findings []string
}

// Review structure is strict while decoded Unicode identity remains lossless.
// docs/adr/0076-go-bounded-loop-controller.md:93.
func parseVerdict(ctx context.Context, response string) (reviewVerdict, error) {
	value, err := jsonvalue.DecodeUnique(ctx, []byte(response))
	if err != nil {
		return reviewVerdict{}, manualError()
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) != 2 {
		return reviewVerdict{}, manualError()
	}
	verdict, ok := object["verdict"].(string)
	if !ok || (verdict != "approve" && verdict != "repair") {
		return reviewVerdict{}, manualError()
	}
	findings, ok := object["findings"].([]any)
	if !ok || len(findings) > 50 || (verdict == "approve") != (len(findings) == 0) {
		return reviewVerdict{}, manualError()
	}
	result := reviewVerdict{Verdict: verdict, Findings: []string{}}
	for _, finding := range findings {
		if err := ctx.Err(); err != nil {
			return reviewVerdict{}, err
		}
		text, ok := finding.(string)
		if !ok || len(pythonFields(text)) == 0 || decodedRuneCount(text) > 4000 {
			return reviewVerdict{}, manualError()
		}
		result.Findings = append(result.Findings, text)
	}
	return result, nil
}
func decodedRuneCount(value string) int {
	count := 0
	for len(value) > 0 {
		_, size := utf8.DecodeRuneInString(value)
		if size == 1 && len(value) >= 3 && value[0] == 0xed && value[1] >= 0xa0 && value[1] <= 0xbf && value[2] >= 0x80 && value[2] <= 0xbf {
			size = 3
		}
		value = value[size:]
		count++
	}
	return count
}

// Decode the bounded raw diagnostic prefix with Python replacement semantics.
// docs/adr/0076-go-bounded-loop-controller.md:89.
func diagnosticText(data []byte) string {
	var result strings.Builder
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r != utf8.RuneError || size != 1 {
			result.Write(data[:size])
			data = data[size:]
			continue
		}
		want := 1
		switch {
		case data[0] >= 0xc2 && data[0] <= 0xdf:
			want = 2
		case data[0] >= 0xe0 && data[0] <= 0xef:
			want = 3
		case data[0] >= 0xf0 && data[0] <= 0xf4:
			want = 4
		}
		consumed := 1
		for consumed < want && consumed < len(data) {
			b := data[consumed]
			if b < 0x80 || b > 0xbf {
				break
			}
			if consumed == 1 && ((data[0] == 0xe0 && b < 0xa0) || (data[0] == 0xed && b >= 0xa0) || (data[0] == 0xf0 && b < 0x90) || (data[0] == 0xf4 && b >= 0x90)) {
				break
			}
			consumed++
		}
		result.WriteRune(utf8.RuneError)
		data = data[consumed:]
	}
	return result.String()
}
