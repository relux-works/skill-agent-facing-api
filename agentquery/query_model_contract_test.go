package agentquery

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/format"
	goparser "go/parser"
	gotoken "go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type frozenContractField struct {
	Name string `json:"name"`
	Type string `json:"type"`
	JSON string `json:"json"`
}

type frozenContractType struct {
	Name           string                `json:"name"`
	Kind           string                `json:"kind"`
	Underlying     string                `json:"underlying"`
	TypeParameters string                `json:"typeParameters"`
	Signature      string                `json:"signature"`
	Fields         []frozenContractField `json:"fields"`
	ExportedFields []frozenContractField `json:"exportedFields"`
	Methods        []string              `json:"methods"`
}

type frozenContractConstant struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type frozenContractFunction struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
}

type frozenContractMethod struct {
	Receiver  string `json:"receiver"`
	Name      string `json:"name"`
	Signature string `json:"signature"`
}

type frozenQueryModelContract struct {
	Grammar struct {
		Productions map[string]string `json:"productions"`
	} `json:"grammar"`
	PublicTypes                 []frozenContractType     `json:"publicTypes"`
	PublicConstants             []frozenContractConstant `json:"publicConstants"`
	PublicFunctions             []frozenContractFunction `json:"publicFunctions"`
	PublicMethods               []frozenContractMethod   `json:"publicMethods"`
	OperationCapabilityCoverage struct {
		RequiredRows          []string          `json:"requiredRows"`
		RequiredCoverageRatio string            `json:"requiredCoverageRatio"`
		RequiredSurfaceRatio  string            `json:"requiredSurfaceRatio"`
		RequiredCaseRatio     string            `json:"requiredCaseRatio"`
		CaseInventory         map[string]string `json:"caseInventory"`
	} `json:"operationCapabilityCoverage"`
	DiscoveryShape struct {
		TopLevelKeys            []string `json:"topLevelKeys"`
		NormalVisibleFields     []string `json:"normalVisibleFields"`
		RestrictedVisibleFields []string `json:"restrictedVisibleFields"`
	} `json:"discoveryShape"`
	ResultShapes       map[string]any `json:"resultShapes"`
	LimitConfiguration struct {
		StateMatrix struct {
			DeclaredCases         []string `json:"declaredCases"`
			RequiredExecutedRatio string   `json:"requiredExecutedRatio"`
		} `json:"stateMatrix"`
	} `json:"limitConfiguration"`
	PublicIdentifierCollisionGate struct {
		BaselinePackageScopeNames []string `json:"baselinePackageScopeNames"`
	} `json:"publicIdentifierCollisionGate"`
}

func loadFrozenQueryModelContract(t *testing.T) frozenQueryModelContract {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", ".spec", "composable-query-expression-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c frozenQueryModelContract
	if err = json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func currentQueryModelDeclarations(t *testing.T) (map[string]bool, map[string]bool) {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	methods := map[string]bool{}
	fs := gotoken.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := goparser.ParseFile(fs, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch x := spec.(type) {
					case *ast.TypeSpec:
						names[x.Name.Name] = true
					case *ast.ValueSpec:
						for _, n := range x.Names {
							names[n.Name] = true
						}
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil {
					names[d.Name.Name] = true
				} else {
					methods[d.Name.Name] = true
				}
			}
		}
	}
	return names, methods
}

func TestQueryModelFrozenContractCompleteness(t *testing.T) {
	c := loadFrozenQueryModelContract(t)
	types, constants, functions, methods := currentFrozenDeclarations(t)
	covered := frozenGrammarCoverage(t, c.Grammar.Productions)
	var missing []string
	for _, v := range c.PublicTypes {
		if declarationMatchesFrozenType(t, types[v.Name], v) {
			covered++
		} else {
			missing = append(missing, v.Name)
		}
	}
	for _, v := range c.PublicConstants {
		if got, ok := constants[v.Name]; ok && got.Type == v.Type && got.Value == v.Value {
			covered++
		} else {
			missing = append(missing, v.Name)
		}
	}
	for _, v := range c.PublicFunctions {
		if got := functions[v.Name]; got != nil && nodeText(t, got.Type) == v.Signature {
			covered++
		} else {
			missing = append(missing, v.Name)
		}
	}
	for _, v := range c.PublicMethods {
		if got := methods[v.Receiver+"."+v.Name]; got != nil && methodSignature(t, got.Type) == v.Signature {
			covered++
		} else {
			missing = append(missing, v.Receiver+"."+v.Name)
		}
	}
	required := len(c.Grammar.Productions) + len(c.PublicTypes) + len(c.PublicConstants) + len(c.PublicFunctions) + len(c.PublicMethods)
	if covered != required {
		sort.Strings(missing)
		t.Fatalf("frozen coverage %d/%d, missing %v", covered, required, missing)
	}
	if required != 156 {
		t.Fatalf("frozen registry changed: %d/156", required)
	}
	t.Logf("frozen coverage %d/%d", covered, required)
}

type frozenConstantDeclaration struct{ Type, Value string }

func currentFrozenDeclarations(t *testing.T) (map[string]*ast.TypeSpec, map[string]frozenConstantDeclaration, map[string]*ast.FuncDecl, map[string]*ast.FuncDecl) {
	t.Helper()
	types := map[string]*ast.TypeSpec{}
	constants := map[string]frozenConstantDeclaration{}
	functions := map[string]*ast.FuncDecl{}
	methods := map[string]*ast.FuncDecl{}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fs := gotoken.NewFileSet()
	for _, filename := range files {
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		file, err := goparser.ParseFile(fs, filename, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					switch item := spec.(type) {
					case *ast.TypeSpec:
						types[item.Name.Name] = item
					case *ast.ValueSpec:
						if value.Tok != gotoken.CONST || item.Type == nil || len(item.Values) != len(item.Names) {
							continue
						}
						for i, name := range item.Names {
							literal := nodeText(t, item.Values[i])
							if unquoted, err := strconv.Unquote(literal); err == nil {
								literal = unquoted
							}
							constants[name.Name] = frozenConstantDeclaration{Type: nodeText(t, item.Type), Value: literal}
						}
					}
				}
			case *ast.FuncDecl:
				if value.Recv == nil {
					functions[value.Name.Name] = value
					continue
				}
				receiver := nodeText(t, value.Recv.List[0].Type)
				methods[receiver+"."+value.Name.Name] = value
			}
		}
	}
	return types, constants, functions, methods
}

func nodeText(t *testing.T, node any) string {
	t.Helper()
	var out bytes.Buffer
	if err := format.Node(&out, gotoken.NewFileSet(), node.(ast.Node)); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func methodSignature(t *testing.T, original *ast.FuncType) string {
	t.Helper()
	copyFields := func(list *ast.FieldList) *ast.FieldList {
		if list == nil {
			return nil
		}
		fields := make([]*ast.Field, 0, len(list.List))
		for _, field := range list.List {
			fields = append(fields, &ast.Field{Type: field.Type})
		}
		return &ast.FieldList{List: fields}
	}
	return nodeText(t, &ast.FuncType{Params: copyFields(original.Params), Results: copyFields(original.Results)})
}

func declarationMatchesFrozenType(t *testing.T, got *ast.TypeSpec, want frozenContractType) bool {
	t.Helper()
	if got == nil {
		return false
	}
	typeParameters := ""
	if got.TypeParams != nil {
		var parameters []string
		for _, field := range got.TypeParams.List {
			for _, name := range field.Names {
				parameters = append(parameters, name.Name+" "+nodeText(t, field.Type))
			}
		}
		typeParameters = strings.Join(parameters, ", ")
	}
	if typeParameters != want.TypeParameters {
		return false
	}
	switch want.Kind {
	case "defined":
		return nodeText(t, got.Type) == want.Underlying
	case "generic-defined-func":
		function, ok := got.Type.(*ast.FuncType)
		return ok && nodeText(t, function) == want.Signature
	case "interface":
		iface, ok := got.Type.(*ast.InterfaceType)
		if !ok {
			return false
		}
		var methods []string
		for _, field := range iface.Methods.List {
			if len(field.Names) != 1 {
				return false
			}
			signature := strings.TrimPrefix(nodeText(t, field.Type), "func")
			methods = append(methods, field.Names[0].Name+signature)
		}
		return reflect.DeepEqual(methods, want.Methods)
	case "struct", "generic-struct", "opaque-generic-struct":
		structure, ok := got.Type.(*ast.StructType)
		if !ok {
			return false
		}
		var fields []frozenContractField
		for _, field := range structure.Fields.List {
			for _, name := range field.Names {
				if !ast.IsExported(name.Name) {
					continue
				}
				jsonTag := ""
				if field.Tag != nil {
					raw, err := strconv.Unquote(field.Tag.Value)
					if err != nil {
						return false
					}
					jsonTag = reflect.StructTag(raw).Get("json")
				}
				fields = append(fields, frozenContractField{Name: name.Name, Type: nodeText(t, field.Type), JSON: jsonTag})
			}
		}
		expected := want.Fields
		if want.Kind == "opaque-generic-struct" {
			expected = want.ExportedFields
		}
		if len(fields) == 0 && len(expected) == 0 {
			return true
		}
		return reflect.DeepEqual(fields, expected)
	default:
		return false
	}
}

func frozenGrammarCoverage(t *testing.T, productions map[string]string) int {
	t.Helper()
	w := map[string]string{
		"query": `list();count()`, "semicolons": `;;list();;`, "statement": `list(a=b) where equals(id,"A") sortOrder(id ascending) groupBy(id) skip 0 take 1 { id }`, "operation": `op-1.x/y()`,
		"legacyArgs": `list(a=b,c)`, "legacyArg": `list(value)`, "legacyKey": `list(key_name=v)`, "legacyValue": `list(k="v")`, "legacyIdentifier": `abc-1.x/y()`, "legacyIdentifierStart": `1op()`, "legacyIdentifierContinue": `a-1.x/y()`, "projection": `list() { id name }`,
		"whereClause": `list() where equals(id,"A")`, "expression": `list() where not(equals(id,"A"))`, "composite": `list() where satisfiesAll(equals(id,"A"),satisfiesAny(isNull(name),isMissing(name)))`, "predicate": `list() where equals(id,"A")`,
		"equals": `list() where equals(id,"A")`, "notEquals": `list() where notEquals(id,"A")`, "contains": `list() where contains(name,"a")`, "matchesRegex": `list() where matchesRegex(name,"^a$")`, "lessThan": `list() where lessThan(score,2)`, "lessThanOrEqual": `list() where lessThanOrEqual(score,2)`, "greaterThan": `list() where greaterThan(score,2)`, "greaterThanOrEqual": `list() where greaterThanOrEqual(score,2)`, "inRange": `list() where inRange(score,1,2)`, "inSet": `list() where inSet(status,["a","b"])`, "hasElement": `list() where hasElement(labels,"a")`, "isNull": `list() where isNull(name)`, "isMissing": `list() where isMissing(name)`,
		"sortClause": `list() sortOrder(id ascending)`, "sortCriterion": `list() sortOrder(id descending)`, "groupClause": `list() groupBy(status)`, "skipClause": `list() skip 1`, "takeClause": `list() take 1`, "setLiteral": `list() where inSet(status,["a"])`, "scalar": `list() where equals(active,true)`, "string": `list() where equals(name,"a\\q")`, "signedInteger": `list() where equals(score,-1)`, "unsignedInteger": `list() skip 0`, "decimalMagnitude": `list() take 10`, "field": `list() where equals(_id1,"A")`,
	}
	covered := 0
	for name := range productions {
		witness, ok := w[name]
		if !ok {
			t.Errorf("missing grammar witness %s", name)
			continue
		}
		model, err := ParseQueryModel(witness, QueryLimits{})
		if err != nil {
			t.Errorf("%s witness: %v", name, err)
			continue
		}
		rendered, err := RenderQueryModel(model)
		if err != nil {
			t.Errorf("%s render: %v", name, err)
			continue
		}
		if _, err = ParseQueryModel(rendered, QueryLimits{}); err != nil {
			t.Errorf("%s round trip: %v", name, err)
			continue
		}
		covered++
	}
	for _, empty := range []string{`list() where satisfiesAll()`, `list() where satisfiesAny()`} {
		if _, err := ParseQueryModel(empty, QueryLimits{}); err == nil {
			t.Errorf("empty composite admitted: %s", empty)
		}
	}
	return covered
}

func collisionNames(candidates, baseline []string) []string {
	set := map[string]bool{}
	for _, v := range baseline {
		set[v] = true
	}
	var out []string
	for _, v := range candidates {
		if set[v] {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func TestQueryModelFrozenPublicSurfaceNoPackageScopeCollision(t *testing.T) {
	c := loadFrozenQueryModelContract(t)
	var candidates []string
	for _, v := range c.PublicTypes {
		candidates = append(candidates, v.Name)
	}
	for _, v := range c.PublicConstants {
		candidates = append(candidates, v.Name)
	}
	for _, v := range c.PublicFunctions {
		candidates = append(candidates, v.Name)
	}
	if got := collisionNames(candidates, c.PublicIdentifierCollisionGate.BaselinePackageScopeNames); len(got) != 0 {
		t.Fatalf("public package-scope collisions: %v", got)
	}
	mutant := append([]string(nil), candidates...)
	for i, v := range mutant {
		if v == "QuerySortDirection" {
			mutant[i] = "SortDirection"
		}
	}
	got := collisionNames(mutant, c.PublicIdentifierCollisionGate.BaselinePackageScopeNames)
	if len(got) != 1 || got[0] != "SortDirection" {
		t.Fatalf("narrowing mutant was not killed: %v", got)
	}
}
