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

// This oracle reads literal data from the immutable trusted Git object. It
// parses only Go syntax and never runs historical source or evaluates expressions.
// per docs/adr/0115-frozen-rc3-recipe-compatibility.md:74
var _ = Describe("Frozen RC3 installation recipe data", func() {
	It("equals all original fingerprint bytes and every ordered literal role field independently of the current image", func() {
		const revision = "95486ecdd3868a319a0e6362ee99cafd8f154200"
		read := func(path string) []byte {
			command := exec.Command("git", "show", revision+":"+path) // #nosec G204 -- immutable evaluator-owned revision and closed literal path list; source remains inert.
			command.Dir = "../.."
			data, err := command.Output()
			Expect(err).NotTo(HaveOccurred(), path)
			return data
		}
		expectedHashes := map[string]string{}
		for _, path := range []string{
			"internal/installation/generate.go", "internal/installation/roles.go",
			"internal/installation/orchestration.go", "internal/installation/reference.go",
			"internal/installation/model.go", "internal/installation/observe.go",
			"internal/installation/selection.go", "internal/installedlayout/layout.go",
		} {
			expectedHashes[path] = fmt.Sprintf("%x", sha256.Sum256(read(path)))
		}
		Expect(recipeV1).To(Equal(expectedHashes))
		source, err := parser.ParseFile(token.NewFileSet(), "immutable-rc3-roles.go", read("internal/installation/roles.go"), 0)
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
					Expect(seen).To(HaveLen(7), "all original role fields must remain in the oracle")
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
		Expect(rolesV1).To(Equal(expected), "all source/path/generator/mode/required/reason fields and ordering remain frozen")
	})
})
