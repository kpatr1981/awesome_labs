package exercise

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNotebookGuide(t *testing.T) {
	g := NotebookGuide(&Exercise{Deliverables: []string{"answer.txt", "clean.csv", "model.joblib"}})
	if len(g) != 3 || !strings.Contains(g[0].Code, `open("/lab/answer.txt", "w")`) ||
		!strings.Contains(g[1].Code, `to_csv("/lab/clean.csv", index=False)`) || !strings.Contains(g[2].Code, `joblib.dump(model, "/lab/model.joblib")`) {
		t.Fatalf("guide: %+v", g)
	}
}

func TestStarterNotebook(t *testing.T) {
	e := &Exercise{ID: "ds-x", Title: "T", Deliverables: []string{"answer.txt"},
		Starter: "def total(df):\n    return df.sum()\n# %%\n\n# %%\nx = total(pd.read_csv('/lab/a.csv'))\n"}
	var nb struct {
		Cells []struct {
			Type   string `json:"cell_type"`
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"cells"`
		Major int `json:"nbformat"`
		Minor int `json:"nbformat_minor"`
	}
	if err := json.Unmarshal(StarterNotebook(e), &nb); err != nil {
		t.Fatal(err)
	}
	if nb.Major != 4 || nb.Minor != 5 {
		t.Fatalf("nbformat %d.%d", nb.Major, nb.Minor)
	}
	var code []string
	ids := map[string]bool{}
	for _, c := range nb.Cells {
		if ids[c.ID] || c.ID == "" {
			t.Fatalf("cell ids must be unique and set: %q", c.ID)
		}
		ids[c.ID] = true
		if c.Type == "code" {
			code = append(code, c.Source)
		}
	}
	if !strings.Contains(nb.Cells[0].Source, "doesn't have `/lab/answer.txt` yet") {
		t.Errorf("header: %q", nb.Cells[0].Source)
	}
	// imports, the two starter cells (the empty one dropped), your analysis, the save cell
	if len(code) != 5 || code[1] != "def total(df):\n    return df.sum()" || !strings.HasPrefix(code[2], "x = total(") {
		t.Fatalf("code cells: %q", code)
	}
}
