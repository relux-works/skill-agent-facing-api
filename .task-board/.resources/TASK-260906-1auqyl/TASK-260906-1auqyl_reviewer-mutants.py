#!/usr/bin/env python3
"""Reviewer-authored narrowing mutants (not declared by the producer)."""
import subprocess, pathlib
ROOT = pathlib.Path("/tmp/aq-review/agentquery")
MUTANTS = [
 ("R1 unknown-op-wins narrowed to the first statement only", "parser.go",
  "\tif config != nil && config.Operations != nil && !config.Operations[name] {",
  "\tif config != nil && config.Operations != nil && !config.Operations[name] && namePos.Offset == 0 {"),
 ("R2 known-op list narrowed: built-in schema hidden", "parser.go",
  "\t\tfor op, accepted := range config.Operations {\n\t\t\tif accepted {",
  "\t\tfor op, accepted := range config.Operations {\n\t\t\tif accepted && op != \"schema\" {"),
 ("R3 tokenizer attribution narrowed to errors without Got", "parser.go",
  "\tpe, ok := err.(*ParseError)\n\tif !ok {\n\t\treturn err\n\t}",
  "\tpe, ok := err.(*ParseError)\n\tif !ok || pe.Got != \"\" {\n\t\treturn err\n\t}"),
 ("R4 Error() narrowed: known list omitted when Got is set", "error.go",
  "\tif len(e.KnownOperations) > 0 {",
  "\tif len(e.KnownOperations) > 0 && e.Got == \"\" {"),
 ("R5 parser-stage unknown-op narrowed: only when the statement has no args", "parser.go",
  "\t\t\treturn nil, NewUnknownOperationError(opTok.val, p.tzer.posAt(opTok.pos), known)",
  "\t\t\tif p.tokens[p.pos+1].typ == tokenRParen {\n\t\t\t\treturn nil, NewUnknownOperationError(opTok.val, p.tzer.posAt(opTok.pos), known)\n\t\t\t}"),
 ("R6 schema() narrowed: grammar loses tokens+escapes, prose kept", "grammar.go",
  "\t\tEscapes:    escapes,",
  "\t\tEscapes:    nil,"),
]
def run():
    p = subprocess.run(["go","test","./...","-count=1"], cwd=ROOT, capture_output=True, text=True)
    fails = sorted({l.split()[2] for l in p.stdout.splitlines() if l.startswith("--- FAIL:")})
    build = "[build failed]" in p.stdout or "build failed" in p.stderr
    return p.returncode, fails, build, p.stdout+p.stderr
rc,_,_,out = run()
print("baseline exit=%d" % rc)
if rc: print(out); raise SystemExit(1)
surv=[]
for name,f,old,new in MUTANTS:
    path=ROOT/f; src=path.read_text()
    if src.count(old)!=1:
        print(f"{name}: ANCHOR NOT UNIQUE (count={src.count(old)})"); surv.append(name); continue
    path.write_text(src.replace(old,new))
    try: rc,fails,build,out = run()
    finally: path.write_text(src)
    if build: print(f"{name}: DID NOT COMPILE"); surv.append(name)
    elif rc==0: print(f"{name}: *** SURVIVED ***"); surv.append(name)
    else: print(f"{name}: KILLED by {', '.join(fails)}")
print("\nSURVIVORS:", surv or "none")
