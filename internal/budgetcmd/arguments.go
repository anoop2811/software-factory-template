package budgetcmd

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/anoop2811/software-factory-template/internal/budget"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var commandNames = [...]string{"plan", "run", "report"}

func rootCommand() *cobra.Command {
	command := &cobra.Command{Use: "factory budget", Short: "Local budget plans, explicit runs, and reports"}
	command.Flags().BoolP("help", "h", false, "Show command help")
	for _, name := range commandNames {
		command.AddCommand(&cobra.Command{Use: name, Short: "Budget " + name, Run: func(*cobra.Command, []string) {}})
	}
	return command
}
func knownCommand(name string) bool {
	for _, candidate := range commandNames {
		if candidate == name {
			return true
		}
	}
	return false
}

type argumentToken struct {
	raw        string
	flag       *pflag.Flag
	explicit   *string
	positional bool
	terminator bool
	short      bool
}

// Classify before executing actions: a later ambiguous prefix cannot be hidden
// by earlier help. Names and arity come from Cobra's flags.
// docs/adr/0072-go-budget-argument-compatibility.md:40.
func classify(ctx context.Context, command *cobra.Command, args []string) ([]argumentToken, error) {
	tokens := make([]argumentToken, 0, len(args))
	ended := false
	for _, raw := range args {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token := argumentToken{raw: raw}
		if ended {
			token.positional = true
			tokens = append(tokens, token)
			continue
		}
		if raw == "--" {
			token.terminator = true
			ended = true
			tokens = append(tokens, token)
			continue
		}
		if !strings.HasPrefix(raw, "-") || raw == "-" {
			token.positional = true
			tokens = append(tokens, token)
			continue
		}
		name, value, attached := strings.Cut(raw, "=")
		if strings.HasPrefix(name, "--") {
			name = strings.TrimPrefix(name, "--")
			token.flag = command.Flags().Lookup(name)
			if token.flag == nil {
				matches := 0
				command.Flags().VisitAll(func(flag *pflag.Flag) {
					if strings.HasPrefix(flag.Name, name) {
						token.flag = flag
						matches++
					}
				})
				if matches > 1 {
					return nil, errors.New("ambiguous option prefix")
				}
			}
			if attached {
				token.explicit = &value
			}
		} else if strings.HasPrefix(raw, "-h") {
			token.flag = command.Flags().ShorthandLookup("h")
			token.short = true
			if raw != "-h" {
				suffix := raw[2:]
				token.explicit = &suffix
			}
		}
		if token.flag == nil && (negativeDecimal(raw) || strings.ContainsRune(raw, ' ')) {
			token.positional = true
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// Python's optional-value grammar accepts negative decimal literals, not generic
// exponent syntax. Equals-form scalar values bypass this classification.
// docs/adr/0072-go-budget-argument-compatibility.md:30.
func negativeDecimal(raw string) bool {
	raw = strings.TrimSuffix(raw, "\n")
	if !strings.HasPrefix(raw, "-") {
		return false
	}
	digits, dot := 0, false
	lastDigit := false
	for _, value := range raw[1:] {
		if value == '.' && !dot {
			dot = true
			lastDigit = false
			continue
		}
		if !unicode.IsDigit(value) {
			return false
		}
		digits++
		lastDigit = true
	}
	return digits > 0 && lastDigit
}
func validHelp(token argumentToken) bool {
	if token.explicit == nil {
		return true
	}
	if !token.short || *token.explicit == "" {
		return false
	}
	return strings.Trim(*token.explicit, "h") == ""
}

func selectAction(ctx context.Context, command *cobra.Command, args []string) (string, []string, bool, bool, error) {
	tokens, err := classify(ctx, command, args)
	if err != nil {
		return "", nil, false, false, err
	}
	deferred := false
	for index, token := range tokens {
		if token.flag != nil {
			if !validHelp(token) {
				return "", nil, false, false, errors.New("help does not accept a value")
			}
			return "", nil, true, false, nil
		}
		if token.positional || token.terminator {
			if !knownCommand(token.raw) {
				return "", nil, false, false, errors.New("expected plan, run, or report command")
			}
			return token.raw, args[index+1:], false, deferred, nil
		}
		deferred = true
	}
	return "", nil, false, false, errors.New("a command is required")
}

func normalizeArguments(ctx context.Context, command *cobra.Command, args []string) ([]string, bool, bool, error) {
	tokens, err := classify(ctx, command, args)
	if err != nil {
		return nil, false, false, err
	}
	normalized := make([]string, 0, len(args))
	deferred := false
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if token.flag == nil {
			deferred = true
			continue
		}
		name := token.flag.Name
		if name == "help" {
			if !validHelp(token) {
				return nil, false, false, errors.New("help does not accept a value")
			}
			return nil, true, false, nil
		}
		if token.flag.NoOptDefVal != "" {
			if token.explicit != nil {
				return nil, false, false, errors.New("option --" + name + " does not accept a value")
			}
			normalized = append(normalized, "--"+name)
			continue
		}
		var value string
		if token.explicit != nil {
			value = *token.explicit
		} else {
			if index+1 == len(tokens) || !tokens[index+1].positional {
				return nil, false, false, errors.New("option --" + name + " requires a value")
			}
			index++
			value = tokens[index].raw
		}
		if name == "harness" && !budget.ValidHarness(value) || name == "role" && !budget.ValidRole(value) {
			return nil, false, false, errors.New("invalid choice for --" + name)
		}
		normalized = append(normalized, "--"+name+"="+value)
	}
	return normalized, false, deferred, nil
}

// Cobra's required annotations drive validation as well as visible help labels.
// docs/adr/0072-go-budget-argument-compatibility.md:109.
func stringOption(command *cobra.Command, target *string, name, fallback, usage string, required bool) error {
	if required {
		usage += " (required)"
	}
	command.Flags().StringVar(target, name, fallback, usage)
	if required {
		return command.MarkFlagRequired(name)
	}
	return nil
}
