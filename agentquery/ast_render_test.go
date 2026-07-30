package agentquery

import (
	"reflect"
	"testing"
)

func TestRenderRoundTripsQuotedSeparators(t *testing.T) {
	input := `set_notes(TASK-1, text="literal; comma, slash\\quote\" and\nnewline"); delete(TASK-2)`

	want, err := Parse(input, nil)
	if err != nil {
		t.Fatalf("Parse input: %v", err)
	}
	rendered := Render(want)
	got, err := Parse(rendered, nil)
	if err != nil {
		t.Fatalf("Parse rendered AST %q: %v", rendered, err)
	}
	clearPositions(got)
	clearPositions(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rendered AST changed\ngot:  %#v\nwant: %#v\nrendered: %s", got, want, rendered)
	}
}

func clearPositions(q *Query) {
	for i := range q.Statements {
		q.Statements[i].Pos = Pos{}
		for j := range q.Statements[i].Args {
			q.Statements[i].Args[j].Pos = Pos{}
		}
	}
}
