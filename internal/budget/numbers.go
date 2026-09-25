package budget

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/anoop2811/software-factory-template/internal/jsonvalue"
)

func finite(f float64) bool { return !math.IsInf(f, 0) && !math.IsNaN(f) }
func integerSyntax(n json.Number) bool {
	return !strings.ContainsAny(string(n), ".eE") && n != "NaN" && n != "Infinity" && n != "-Infinity"
}
func exactInteger(value any) (*big.Int, bool) {
	n, ok := value.(json.Number)
	if !ok || !integerSyntax(n) {
		return nil, false
	}
	i, ok := new(big.Int).SetString(string(n), 10)
	return i, ok
}
func checkedInteger(value any, zero bool) (*big.Int, bool) {
	n, ok := exactInteger(value)
	if !ok || n.Sign() < 0 || (!zero && n.Sign() == 0) {
		return nil, false
	}
	f, _ := n.Float64()
	return n, finite(f)
}
func numericFloat(value any) (float64, bool) {
	n, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil && finite(f)
}
func floatNumber(f float64) json.Number {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return json.Number(s)
}
func integerNumber(i *big.Int) json.Number { return json.Number(i.String()) }

// CPython 3.12 switches permanently to generic addition after integer overflow.
// Its float fast path compensates float operands, but not C-long integers.
// docs/adr/0069-go-budget-ledger-admission.md:39.
type numberSum struct {
	integer           big.Int
	floating          bool
	fast              bool
	total, correction float64
}

func newSum() *numberSum { return &numberSum{fast: true} }
func (s *numberSum) add(value any) error {
	n, ok := value.(json.Number)
	if !ok {
		return errors.New("invalid budget aggregate")
	}
	if i, integer := exactInteger(n); integer {
		if !s.floating {
			s.integer.Add(&s.integer, i)
			if !i.IsInt64() || !s.integer.IsInt64() {
				s.fast = false
			}
			return nil
		}
		x, _ := i.Float64()
		if !finite(x) {
			return errors.New("budget aggregate overflow")
		}
		if s.fast && !i.IsInt64() {
			s.finishCompensation()
			s.fast = false
		}
		s.total += x
	} else {
		x, ok := numericFloat(n)
		if !ok {
			return errors.New("invalid budget aggregate")
		}
		switch {
		case !s.floating:
			base, _ := s.integer.Float64()
			if !finite(base) {
				return errors.New("budget aggregate overflow")
			}
			s.total = base + x
			s.floating = true
		case s.fast:
			t := s.total + x
			if math.Abs(s.total) >= math.Abs(x) {
				s.correction += (s.total - t) + x
			} else {
				s.correction += (x - t) + s.total
			}
			s.total = t
		default:
			s.total += x
		}
	}
	if !finite(s.total) {
		return errors.New("budget aggregate overflow")
	}
	return nil
}
func (s *numberSum) finishCompensation() {
	if s.correction != 0 && finite(s.correction) {
		s.total += s.correction
	}
	s.correction = 0
}
func (s *numberSum) number() (json.Number, error) {
	if !s.floating {
		return integerNumber(&s.integer), nil
	}
	value := s.total
	if s.correction != 0 && finite(s.correction) {
		value += s.correction
	}
	if !finite(value) {
		return "", errors.New("budget aggregate overflow")
	}
	return floatNumber(value), nil
}
func (s *numberSum) float() (float64, error) {
	n, err := s.number()
	if err != nil {
		return 0, err
	}
	f, ok := numericFloat(n)
	if !ok {
		return 0, errors.New("budget aggregate overflow")
	}
	return f, nil
}
func (s *numberSum) atLeast(threshold float64) (bool, error) {
	if !s.floating {
		r := new(big.Rat).SetFloat64(threshold)
		return new(big.Rat).SetInt(&s.integer).Cmp(r) >= 0, nil
	}
	f, err := s.float()
	return f >= threshold, err
}

func pythonFloat(raw string) (float64, error) {
	value, err := jsonvalue.PositiveFloat(raw)
	if err != nil {
		return 0, errors.New("invalid budget number")
	}
	return value, nil
}
