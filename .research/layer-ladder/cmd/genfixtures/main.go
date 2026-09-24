// Command genfixtures renders the layer ladder for every scale and writes the
// fixtures plus a manifest the Python measurement consumes.
//
// Usage (from .research/layer-ladder):
//
//	go run ./cmd/genfixtures
//	go run ./cmd/genfixtures -out fixtures -payloads ../synthetic-payloads
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	ladder "github.com/relux-works/skill-agent-facing-api/research/layerladder"
)

type fixtureEntry struct {
	Scale          int            `json:"scale"`
	Layer          ladder.LayerID `json:"layer"`
	Name           string         `json:"name"`
	Path           string         `json:"path"`
	Bytes          int            `json:"bytes"`
	ScenarioDigest string         `json:"scenario_digest"`
}

type schemaArtifact struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
	Note  string `json:"note"`
}

type manifest struct {
	Generator       string           `json:"generator"`
	Scenario        string           `json:"scenario"`
	ScenarioQuery   string           `json:"scenario_query"`
	FullQuery       string           `json:"full_query"`
	FullFields      []string         `json:"full_fields"`
	ScenarioFields  []string         `json:"scenario_fields"`
	SourcePayloads  string           `json:"source_payloads"`
	Scales          []int            `json:"scales"`
	Layers          []ladder.Layer   `json:"layers"`
	Fixtures        []fixtureEntry   `json:"fixtures"`
	SchemaArtifacts []schemaArtifact `json:"schema_artifacts"`
}

func main() {
	out := flag.String("out", "fixtures", "output directory for rendered fixtures")
	payloads := flag.String("payloads", filepath.Join("..", "synthetic-payloads"), "directory holding the checked-in json-N.txt fixtures")
	flag.Parse()

	if err := run(*out, *payloads); err != nil {
		fmt.Fprintln(os.Stderr, "genfixtures:", err)
		os.Exit(1)
	}
}

func run(outDir, payloadDir string) error {
	m := manifest{
		Generator:      "research/layer-ladder/cmd/genfixtures",
		Scenario:       "Sprint stand-up roll-call: the agent needs id, status, assignee and priority for every task on the sprint page. Name, description, created and updated are carried by the record but not needed.",
		ScenarioQuery:  ladder.ScenarioQuery,
		FullQuery:      ladder.FullQuery,
		FullFields:     ladder.FullFields,
		ScenarioFields: ladder.ScenarioFields,
		SourcePayloads: payloadDir,
		Scales:         ladder.Scales,
		Layers:         ladder.Layers,
	}

	for _, scale := range ladder.Scales {
		tasks, err := ladder.LoadTasks(payloadDir, scale)
		if err != nil {
			return err
		}
		canonical := ladder.CanonicalRecords(tasks)
		digest := ladder.ScenarioDigest(canonical)

		schema := ladder.NewSchema(tasks)
		rendered, err := ladder.Render(schema)
		if err != nil {
			return err
		}

		scaleDir := filepath.Join(outDir, fmt.Sprintf("%d", scale))
		if err := os.MkdirAll(scaleDir, 0o755); err != nil {
			return err
		}

		for _, layer := range ladder.Layers {
			payload, ok := rendered[layer.ID]
			if !ok {
				return fmt.Errorf("scale %d: layer %s was not rendered", scale, layer.ID)
			}
			// The preservation gate runs before anything is written, so a
			// fixture that lost scenario data can never reach measurement.
			if err := ladder.VerifyPreservation(layer.ID, payload, canonical); err != nil {
				return err
			}
			decoded, err := ladder.DecodeScenarioFields(layer.ID, payload)
			if err != nil {
				return err
			}
			if got := ladder.ScenarioDigest(decoded); got != digest {
				return fmt.Errorf("scale %d layer %s: scenario digest %s, want %s", scale, layer.ID, got, digest)
			}

			name := fmt.Sprintf("%s-%s.txt", layer.ID, layer.Name)
			path := filepath.Join(scaleDir, name)
			if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
				return err
			}
			m.Fixtures = append(m.Fixtures, fixtureEntry{
				Scale:          scale,
				Layer:          layer.ID,
				Name:           layer.Name,
				Path:           filepath.ToSlash(path),
				Bytes:          len(payload),
				ScenarioDigest: digest,
			})
		}
	}

	// Session-constant artifacts: what the agent pays once to learn the contract.
	schema := ladder.NewSchema(nil)
	schemaJSON, err := schema.QueryJSON("schema()")
	if err != nil {
		return fmt.Errorf("render schema(): %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	schemaPath := filepath.Join(outDir, "schema-response.json")
	if err := os.WriteFile(schemaPath, schemaJSON, 0o644); err != nil {
		return err
	}
	m.SchemaArtifacts = append(m.SchemaArtifacts, schemaArtifact{
		Name:  "agentquery-schema-response",
		Path:  filepath.ToSlash(schemaPath),
		Bytes: len(schemaJSON),
		Note:  "Real Schema.QueryJSON(\"schema()\") output for this scenario's contract. Paid once per session if the agent introspects at all.",
	})

	legend := aliasLegend()
	legendPath := filepath.Join(outDir, "alias-legend.txt")
	if err := os.WriteFile(legendPath, []byte(legend), 0o644); err != nil {
		return err
	}
	m.SchemaArtifacts = append(m.SchemaArtifacts, schemaArtifact{
		Name:  "alias-legend",
		Path:  filepath.ToSlash(legendPath),
		Bytes: len(legend),
		Note:  "Research overlay only. The one-time legend an agent would need to read L5. agentquery ships no alias support.",
	})

	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(outDir, "manifest.json")
	if err := os.WriteFile(manifestPath, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %d fixtures, %d schema artifacts, manifest %s\n", len(m.Fixtures), len(m.SchemaArtifacts), manifestPath)
	return nil
}

func aliasLegend() string {
	out := "compact header aliases:\n"
	for _, f := range ladder.ScenarioFields {
		out += fmt.Sprintf("%s = %s\n", ladder.AliasHeader[f], f)
	}
	return out
}
