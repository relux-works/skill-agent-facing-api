package layerladder

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/relux-works/skill-agent-facing-api/agentquery"
)

const payloadDir = "../synthetic-payloads"

func loadScale(t *testing.T, scale int) ([]Task, []map[string]string, *agentquery.Schema[Task]) {
	t.Helper()
	tasks, err := LoadTasks(payloadDir, scale)
	if err != nil {
		t.Fatalf("LoadTasks(%d): %v", scale, err)
	}
	return tasks, CanonicalRecords(tasks), NewSchema(tasks)
}

// Positive control: every rendered layer, at every measured scale, still answers
// the scenario question with exactly the canonical records. Without this the
// negative tests below could be satisfied by a renderer that rejects everything.
func TestRenderedLayersPreserveScenarioData(t *testing.T) {
	for _, scale := range Scales {
		tasks, canonical, schema := loadScale(t, scale)
		if len(tasks) != scale {
			t.Fatalf("scale %d: loaded %d tasks", scale, len(tasks))
		}
		rendered, err := Render(schema)
		if err != nil {
			t.Fatalf("Render(scale=%d): %v", scale, err)
		}
		if len(rendered) != len(Layers) {
			t.Fatalf("scale %d: rendered %d layers, want %d", scale, len(rendered), len(Layers))
		}
		for _, layer := range Layers {
			if err := VerifyPreservation(layer.ID, rendered[layer.ID], canonical); err != nil {
				t.Errorf("scale %d layer %s: %v", scale, layer.ID, err)
			}
		}
	}
}

// The projected layers must actually be the shipped agentquery output, not a
// measurement-local re-render. Drives Schema.QueryJSONWithMode directly and
// compares byte-for-byte with what Render wrote.
func TestProjectedLayersComeFromProductionQueryPath(t *testing.T) {
	_, _, schema := loadScale(t, 20)
	rendered, err := Render(schema)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	cases := []struct {
		layer LayerID
		query string
		mode  agentquery.OutputMode
	}{
		{L1JSONMinFull, FullQuery, agentquery.HumanReadable},
		{L3JSONMinProjected, ScenarioQuery, agentquery.HumanReadable},
		{L4CompactProjected, ScenarioQuery, agentquery.LLMReadable},
	}
	for _, tc := range cases {
		want, err := schema.QueryJSONWithMode(tc.query, tc.mode)
		if err != nil {
			t.Fatalf("layer %s: QueryJSONWithMode: %v", tc.layer, err)
		}
		if rendered[tc.layer] != string(want) {
			t.Errorf("layer %s does not match production output of %q", tc.layer, tc.query)
		}
	}
}

// Projection must remove exactly the unrequested fields and nothing else.
func TestProjectionDropsOnlyUnrequestedFields(t *testing.T) {
	_, _, schema := loadScale(t, 5)
	rendered, err := Render(schema)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, dropped := range []string{"name", "description", "created", "updated"} {
		if !strings.Contains(rendered[L1JSONMinFull], `"`+dropped+`"`) {
			t.Errorf("full layer L1 unexpectedly lacks field %q", dropped)
		}
		if strings.Contains(rendered[L3JSONMinProjected], `"`+dropped+`"`) {
			t.Errorf("projected layer L3 still carries unrequested field %q", dropped)
		}
	}
	for _, kept := range ScenarioFields {
		if !strings.Contains(rendered[L3JSONMinProjected], `"`+kept+`"`) {
			t.Errorf("projected layer L3 lost requested field %q", kept)
		}
	}
}

// --- Preservation gate: negative tests -------------------------------------
//
// Each case is a distinct member of the class the gate must reject. A gate that
// only notices a truncated payload would pass a corrupted one.

func TestVerifyPreservationRejectsDroppedRecord(t *testing.T) {
	_, canonical, schema := loadScale(t, 20)
	rendered, _ := Render(schema)
	payload := rendered[L4CompactProjected]
	lines := strings.Split(strings.TrimRight(payload, "\n"), "\n")
	truncated := strings.Join(lines[:len(lines)-1], "\n") + "\n"

	if err := VerifyPreservation(L4CompactProjected, truncated, canonical); err == nil {
		t.Fatal("gate admitted a payload missing its last record")
	} else if !errors.Is(err, ErrPreservation) {
		t.Fatalf("want ErrPreservation, got %v", err)
	}
}

func TestVerifyPreservationRejectsAlteredLastRecord(t *testing.T) {
	_, canonical, schema := loadScale(t, 20)
	rendered, _ := Render(schema)
	lines := strings.Split(strings.TrimRight(rendered[L4CompactProjected], "\n"), "\n")
	last := len(lines) - 1
	cells := strings.Split(lines[last], ",")
	cells[0] = "TASK-9999"
	lines[last] = strings.Join(cells, ",")
	tampered := strings.Join(lines, "\n") + "\n"

	if err := VerifyPreservation(L4CompactProjected, tampered, canonical); err == nil {
		t.Fatal("gate admitted a payload whose last record id was rewritten")
	} else if !errors.Is(err, ErrPreservation) {
		t.Fatalf("want ErrPreservation, got %v", err)
	}
}

func TestVerifyPreservationRejectsAlteredPriorityValue(t *testing.T) {
	_, canonical, schema := loadScale(t, 20)
	rendered, _ := Render(schema)
	lines := strings.Split(strings.TrimRight(rendered[L4CompactProjected], "\n"), "\n")
	cells := strings.Split(lines[3], ",")
	// Column 3 is "priority" in the scenario header order.
	if cells[3] == "low" {
		cells[3] = "critical"
	} else {
		cells[3] = "low"
	}
	lines[3] = strings.Join(cells, ",")
	tampered := strings.Join(lines, "\n") + "\n"

	if err := VerifyPreservation(L4CompactProjected, tampered, canonical); err == nil {
		t.Fatal("gate admitted a payload whose priority value was rewritten")
	} else if !errors.Is(err, ErrPreservation) {
		t.Fatalf("want ErrPreservation, got %v", err)
	}
}

func TestVerifyPreservationRejectsReorderedRecords(t *testing.T) {
	_, canonical, schema := loadScale(t, 20)
	rendered, _ := Render(schema)
	lines := strings.Split(strings.TrimRight(rendered[L4CompactProjected], "\n"), "\n")
	lines[1], lines[2] = lines[2], lines[1]
	reordered := strings.Join(lines, "\n") + "\n"

	if err := VerifyPreservation(L4CompactProjected, reordered, canonical); err == nil {
		t.Fatal("gate admitted a payload whose record order changed")
	}
}

func TestVerifyPreservationRejectsJSONLayerMissingScenarioField(t *testing.T) {
	_, canonical, _ := loadScale(t, 5)
	// Same records, but "priority" was never serialized.
	payload := `[{"id":"TASK-0001","status":"in-progress","assignee":"heidi"}]`
	if err := VerifyPreservation(L3JSONMinProjected, payload, canonical); err == nil {
		t.Fatal("gate admitted a JSON layer missing a scenario field")
	}
}

func TestVerifyPreservationRejectsCompactRowWithWrongColumnCount(t *testing.T) {
	_, canonical, schema := loadScale(t, 5)
	rendered, _ := Render(schema)
	lines := strings.Split(strings.TrimRight(rendered[L4CompactProjected], "\n"), "\n")
	lines[2] = lines[2] + ",extra"
	broken := strings.Join(lines, "\n") + "\n"

	if err := VerifyPreservation(L4CompactProjected, broken, canonical); err == nil {
		t.Fatal("gate admitted a compact row with more values than header columns")
	}
}

func TestVerifyPreservationRejectsAliasLayerWithUnknownHeader(t *testing.T) {
	_, canonical, schema := loadScale(t, 5)
	rendered, _ := Render(schema)
	lines := strings.Split(strings.TrimRight(rendered[L5CompactAliasProjected], "\n"), "\n")
	lines[0] = "i,s,a,x" // "x" was never registered as an alias
	broken := strings.Join(lines, "\n") + "\n"

	if err := VerifyPreservation(L5CompactAliasProjected, broken, canonical); err == nil {
		t.Fatal("gate admitted an alias header that cannot be mapped back to fields")
	}
}

// --- Alias gate: negative tests --------------------------------------------

func TestApplyAliasHeaderPreservesDataRows(t *testing.T) {
	_, _, schema := loadScale(t, 20)
	rendered, _ := Render(schema)
	compact := rendered[L4CompactProjected]
	alias := rendered[L5CompactAliasProjected]

	compactBody := compact[strings.IndexByte(compact, '\n'):]
	aliasBody := alias[strings.IndexByte(alias, '\n'):]
	if compactBody != aliasBody {
		t.Fatal("alias overlay changed data rows; only the header row may differ")
	}
	if strings.SplitN(alias, "\n", 2)[0] != "i,s,a,p" {
		t.Fatalf("unexpected alias header %q", strings.SplitN(alias, "\n", 2)[0])
	}
}

func TestApplyAliasHeaderRejectsUnregisteredColumn(t *testing.T) {
	_, err := ApplyAliasHeader("id,status,assignee,owner\nTASK-1,open,heidi,x\n", AliasHeader)
	if err == nil {
		t.Fatal("gate admitted a header column with no registered alias")
	}
	if !errors.Is(err, ErrAlias) {
		t.Fatalf("want ErrAlias, got %v", err)
	}
}

func TestApplyAliasHeaderRejectsDuplicateAlias(t *testing.T) {
	clashing := map[string]string{"id": "x", "status": "x", "assignee": "a", "priority": "p"}
	_, err := ApplyAliasHeader("id,status,assignee,priority\nTASK-1,open,heidi,low\n", clashing)
	if err == nil {
		t.Fatal("gate admitted two columns collapsed onto one alias")
	}
	if !errors.Is(err, ErrAlias) {
		t.Fatalf("want ErrAlias, got %v", err)
	}
}

func TestApplyAliasHeaderRejectsNonShorteningAlias(t *testing.T) {
	useless := map[string]string{"id": "id", "status": "s", "assignee": "a", "priority": "p"}
	_, err := ApplyAliasHeader("id,status,assignee,priority\nTASK-1,open,heidi,low\n", useless)
	if err == nil {
		t.Fatal("gate admitted an alias that does not shorten its column")
	}
}

func TestApplyAliasHeaderRejectsHeaderlessPayload(t *testing.T) {
	if _, err := ApplyAliasHeader("id,status,assignee,priority", AliasHeader); err == nil {
		t.Fatal("gate admitted a payload with no header row terminator")
	}
	if _, err := ApplyAliasHeader("", AliasHeader); err == nil {
		t.Fatal("gate admitted an empty payload")
	}
}

// --- Decoder and digest ----------------------------------------------------

func TestDecodeCompactLayerHandlesQuotedValues(t *testing.T) {
	payload := "id,status,assignee,priority\n" +
		`TASK-1,"open, waiting",heidi,low` + "\n" +
		`TASK-2,"says ""ok""",rosa,high` + "\n"
	got, err := DecodeScenarioFields(L4CompactProjected, payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got[0]["status"] != "open, waiting" {
		t.Errorf("quoted comma value decoded as %q", got[0]["status"])
	}
	if got[1]["status"] != `says "ok"` {
		t.Errorf("doubled-quote value decoded as %q", got[1]["status"])
	}
}

func TestScenarioDigestDistinguishesLateRecordChange(t *testing.T) {
	_, canonical, _ := loadScale(t, 100)
	base := ScenarioDigest(canonical)

	altered := make([]map[string]string, len(canonical))
	for i, r := range canonical {
		cp := make(map[string]string, len(r))
		for k, v := range r {
			cp[k] = v
		}
		altered[i] = cp
	}
	altered[len(altered)-1]["assignee"] = "nobody"

	if ScenarioDigest(altered) == base {
		t.Fatal("digest did not change when the last record's assignee changed")
	}
}

func TestLoadTasksRejectsMissingFixture(t *testing.T) {
	if _, err := LoadTasks(filepath.Join(payloadDir, "does-not-exist"), 5); err == nil {
		t.Fatal("LoadTasks accepted a missing payload directory")
	}
}

// A clash between two columns that are not the first column must be caught too:
// a gate that only remembers the first column's alias would admit this.
func TestApplyAliasHeaderRejectsDuplicateAliasBetweenLaterColumns(t *testing.T) {
	clashing := map[string]string{"id": "i", "status": "z", "assignee": "z", "priority": "p"}
	_, err := ApplyAliasHeader("id,status,assignee,priority\nTASK-1,open,heidi,low\n", clashing)
	if err == nil {
		t.Fatal("gate admitted a clash between the second and third columns")
	}
	if !errors.Is(err, ErrAlias) {
		t.Fatalf("want ErrAlias, got %v", err)
	}
}
