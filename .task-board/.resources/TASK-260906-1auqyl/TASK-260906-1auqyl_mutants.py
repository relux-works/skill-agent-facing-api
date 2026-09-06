#!/usr/bin/env python3
"""Narrowing-mutant harness for the DSL error-guidance delta (TASK-260906-1auqyl).

Each mutant keeps the gate in place and weakens it so it admits exactly one
member of the class it must reject. The executor is the behavioral suite
(go test ./... -count=1), not a static checker.
"""
import subprocess, sys, pathlib

ROOT = pathlib.Path(__file__).resolve().parents[2] / ".temp/STORY-260906-1mz2ft/worktree/agentquery"

MUTANTS = [
    ("M1 unknown-op-wins loses batch statements",
     "parser.go",
     "\t\tcase t.typ == tokenSemicolon:\n\t\t\tatStart = true",
     "\t\tcase t.typ == tokenSemicolon:\n\t\t\tatStart = false"),

    ("M2 parser-stage attribution skips errors that carry Expected",
     "parser.go",
     'if errors.As(err, &parseErr) && parseErr.Operation == "" {',
     'if errors.As(err, &parseErr) && parseErr.Operation == "" && parseErr.Expected == "" {'),

    ("M3 ValidateAST reports unknown op without the known list / hint",
     "query.go",
     "return NewUnknownOperationError(statement.Operation, statement.Pos, s.operationNames())",
     'return &ParseError{Message: "unknown operation \\"" + statement.Operation + "\\"", Pos: statement.Pos, Got: statement.Operation}'),

    ("M4 unknown-op error drops the schema() recovery pointer",
     "error.go",
     "\t\tHint:            UnknownOperationHint,",
     '\t\tHint:            "",'),

    ("M4b unknown-op known list is not sorted",
     "error.go",
     "\tsort.Strings(sorted)",
     "\t_ = sort.Strings"),

    ("M5 token name ';' preserved, tokenizer stops emitting it",
     "parser.go",
     "\t\tcase ';':\n\t\t\tt.emit(tokenSemicolon, \";\")",
     "\t\tcase ';':\n\t\t\tt.pos++\n\t\t\tcontinue"),

    ("M6 schema() keeps the grammar key but empties it",
     "schema.go",
     '"grammar":       DSLGrammar(),',
     '"grammar":       Grammar{},'),

    ("M7 escape table decodes \\n to 'x' instead of newline",
     "grammar.go",
     "\t'n':  '\\n',",
     "\t'n':  'x',"),

    ("M8 renderer stops escaping tab",
     "render.go",
     "\t\tif encoded, ok := stringUnescapes[value[i]]; ok {",
     "\t\tif encoded, ok := stringUnescapes[value[i]]; ok && value[i] != '\\t' {"),
]


def run_tests():
    p = subprocess.run(["go", "test", "./...", "-count=1"], cwd=ROOT,
                       capture_output=True, text=True)
    fails = sorted({l.split()[2] for l in p.stdout.splitlines()
                    if l.startswith("--- FAIL:")})
    build_err = "[build failed]" in p.stdout or "build failed" in p.stderr
    return p.returncode, fails, build_err, p.stdout + p.stderr


def main():
    rc, fails, _, out = run_tests()
    if rc != 0:
        print("BASELINE IS NOT GREEN:\n" + out)
        return 1
    print("baseline: go test ./... -count=1 exit=0\n")
    survivors = []
    for name, fname, old, new in MUTANTS:
        path = ROOT / fname
        src = path.read_text()
        if src.count(old) != 1:
            print(f"{name}: ANCHOR NOT UNIQUE in {fname} (count={src.count(old)}) -- NOT APPLIED")
            survivors.append((name, "anchor not applied"))
            continue
        path.write_text(src.replace(old, new))
        try:
            rc, fails, build_err, out = run_tests()
        finally:
            path.write_text(src)
        if build_err:
            print(f"{name}: mutant does not compile -- not valid evidence")
            survivors.append((name, "did not compile"))
        elif rc == 0:
            print(f"{name}: SURVIVED (suite still green)")
            survivors.append((name, "survived"))
        else:
            print(f"{name}: KILLED by {', '.join(fails) if fails else '(non-test failure)'}")
    print()
    if survivors:
        print("SURVIVORS / INVALID:")
        for n, why in survivors:
            print(f"  - {n}: {why}")
    else:
        print("no survivors: every mutant killed by at least one named test")
    return 0


if __name__ == "__main__":
    sys.exit(main())
