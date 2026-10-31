package installation

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func immutableRecipeSource(revision, path string) []byte {
	GinkgoHelper()
	command := exec.Command("git", "--no-replace-objects", "show", revision+":"+path) // #nosec G204 -- immutable evaluator-owned revisions and closed literal source paths; never execution.
	command.Dir = "../.."
	data, err := command.Output()
	Expect(err).NotTo(HaveOccurred(), path)
	return data
}

// Parse inert literal fields rather than evaluating old code or asking the
// current generator to describe its predecessor.
// per docs/adr/0119-minimal-qualified-initializer-inventory.md:52
func immutableRecipeRoles(revision string) []role {
	GinkgoHelper()
	source, err := parser.ParseFile(token.NewFileSet(), "immutable-role-oracle.go", immutableRecipeSource(revision, "internal/installation/roles.go"), 0)
	Expect(err).NotTo(HaveOccurred())
	expected := []role{}
	declarations := 0
	for _, decl := range source.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != "roles" {
				continue
			}
			declarations++
			Expect(value.Values).To(HaveLen(1))
			literal, ok := value.Values[0].(*ast.CompositeLit)
			Expect(ok).To(BeTrue())
			for _, element := range literal.Elts {
				row, ok := element.(*ast.CompositeLit)
				Expect(ok).To(BeTrue())
				oracle := role{}
				seen := map[string]bool{}
				for _, field := range row.Elts {
					pair, ok := field.(*ast.KeyValueExpr)
					Expect(ok).To(BeTrue())
					key, ok := pair.Key.(*ast.Ident)
					Expect(ok).To(BeTrue())
					Expect(seen).NotTo(HaveKey(key.Name))
					seen[key.Name] = true
					if key.Name == "Required" {
						boolean, ok := pair.Value.(*ast.Ident)
						Expect(ok).To(BeTrue())
						Expect(boolean.Name).To(Or(Equal("true"), Equal("false")))
						oracle.Required = boolean.Name == "true"
						continue
					}
					scalar, ok := pair.Value.(*ast.BasicLit)
					Expect(ok).To(BeTrue())
					Expect(scalar.Kind).To(Equal(token.STRING))
					text, err := strconv.Unquote(scalar.Value)
					Expect(err).NotTo(HaveOccurred())
					switch key.Name {
					case "Source":
						oracle.Source = text
					case "Path":
						oracle.Path = text
					case "Operation":
						oracle.Operation = text
					case "Generator":
						oracle.Generator = text
					case "Mode":
						oracle.Mode = text
					case "Reason":
						oracle.Reason = text
					default:
						Fail("unsupported historical literal field: " + key.Name)
					}
				}
				Expect(seen).To(HaveLen(7), "all original role fields remain in the oracle")
				expected = append(expected, oracle)
			}
		}
	}
	Expect(declarations).To(Equal(1))
	Expect(expected).To(HaveLen(141))
	cached := 0
	for _, row := range expected {
		if row.Operation == "inert-data" {
			cached++
		}
	}
	Expect(cached).To(Equal(85))
	return expected
}

func immutableRecipeHashes(revision string, paths []string) map[string]string {
	GinkgoHelper()
	hashes := map[string]string{}
	for _, path := range paths {
		hashes[path] = fmt.Sprintf("%x", sha256.Sum256(immutableRecipeSource(revision, path)))
	}
	return hashes
}

var _ = Describe("Historical installation recipe integrity", func() {
	// per docs/adr/0115-frozen-rc3-recipe-compatibility.md:74
	It("equals all original fingerprint bytes and every ordered literal role field independently of the current image", func() {
		const revision = "95486ecdd3868a319a0e6362ee99cafd8f154200"
		paths := []string{"internal/installation/generate.go", "internal/installation/roles.go", "internal/installation/orchestration.go", "internal/installation/reference.go", "internal/installation/model.go", "internal/installation/observe.go", "internal/installation/selection.go", "internal/installedlayout/layout.go"}
		Expect(recipeV1).To(Equal(immutableRecipeHashes(revision, paths)))
		Expect(rolesV1).To(Equal(immutableRecipeRoles(revision)), "all source/path/generator/mode/required/reason fields and ordering remain frozen")
	})

	// per docs/adr/0119-minimal-qualified-initializer-inventory.md:35
	It("binds the complete later predecessor closure to the independently read original role vector", func() {
		const revision = "9be9d0333d018c51dfccea18da966e6138e380ab"
		paths := []string{"internal/installation/generate.go", "internal/installation/roles.go", "internal/installation/orchestration.go", "internal/installation/reference.go", "internal/installation/model.go", "internal/installation/observe.go", "internal/installation/selection.go", "internal/installedlayout/layout.go", "internal/installation/recipe.go", "internal/installation/roles_v1.go"}
		Expect(recipeV2).To(Equal(immutableRecipeHashes(revision, paths)), "all ten original source members are required")
		Expect(rolesV1).To(Equal(immutableRecipeRoles(revision)), "the shared table must equal every independently read immutable role field")
	})
	// per docs/adr/0119-minimal-qualified-initializer-inventory.md:20
	It("retains every original noncache role and precisely the two consumed cache roles", func() {
		original := immutableRecipeRoles("9be9d0333d018c51dfccea18da966e6138e380ab")
		expected := []role{}
		oldActive, newActive := 0, 0
		retired := []string{}
		for _, row := range original {
			if row.Operation != "retire" {
				oldActive++
			} else {
				retired = append(retired, row.Path)
			}
			if row.Operation == "inert-data" && row.Source != "templates/metrics.html" && row.Source != "packs/review-lane/review-pr.yml" {
				continue
			}
			expected = append(expected, row)
			if row.Operation != "retire" {
				newActive++
			}
		}
		Expect(roles).To(Equal(expected), "no active or retirement role changes accompany the 83 removed cache declarations")
		Expect(expected).To(HaveLen(58))
		Expect(retired).To(ConsistOf("scripts/lib/budget.py", "scripts/lib/budget_adapters.py", "scripts/lib/loop.py"))
		Expect(oldActive).To(Equal(138))
		Expect(newActive).To(Equal(55))
		Expect(oldActive - 2).To(Equal(136))
		Expect(newActive - 2).To(Equal(53))
	})

})
