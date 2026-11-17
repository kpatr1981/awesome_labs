package exercise

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// Notebook exercises (a module's notebook: true) are done in a Jupyter
// notebook, /lab/<id>.ipynb, which `start` writes. The check runs that notebook
// from top to bottom in a fresh kernel, on a copy of /lab without the
// exercise's deliverables, and grades what the notebook wrote (lib.sh
// nb_check); when the task asks for functions, it then calls them on its own
// data. Files you write by hand in /lab therefore don't count.

// NotebookName is the exercise's notebook in /lab.
func (e *Exercise) NotebookName() string { return e.ID + ".ipynb" }

// GuideItem says how a notebook saves one deliverable.
type GuideItem struct {
	File string `json:"file"`
	How  string `json:"how"`
	Code string `json:"code"`
}

// NotebookGuide is a save step for each of the exercise's deliverables.
func NotebookGuide(e *Exercise) []GuideItem {
	var out []GuideItem
	for _, d := range e.Deliverables {
		p := "/lab/" + d
		it := GuideItem{File: d}
		switch path.Ext(d) {
		case ".txt":
			it.How, it.Code = "Write it in the format the scenario asks: just the value, or one value per line.",
				"with open(\""+p+"\", \"w\") as f:\n    f.write(f\"{answer}\\n\")  # answer: your result"
		case ".csv":
			it.How, it.Code = "Save the DataFrame with the columns the scenario asks for (usually without the index).",
				"df.to_csv(\""+p+"\", index=False)  # df: your result"
		case ".joblib":
			it.How, it.Code = "Save the fitted model with joblib.", "import joblib\njoblib.dump(model, \""+p+"\")  # model: your fitted estimator"
		case ".pkl":
			it.How, it.Code = "Save the DataFrame with to_pickle.", "df.to_pickle(\""+p+"\")  # df: your result"
		case ".json":
			it.How, it.Code = "Save it as JSON.", "import json\njson.dump(result, open(\""+p+"\", \"w\"))  # result: your result"
		default:
			it.How = "Write it from the notebook to " + p + "."
		}
		out = append(out, it)
	}
	return out
}

// StarterCells splits the exercise's starter code into cells at lines that
// are just "# %%".
func StarterCells(src string) []string {
	var cells, cur []string
	flush := func() {
		if c := strings.Trim(strings.Join(cur, "\n"), "\n"); strings.TrimSpace(c) != "" {
			cells = append(cells, c)
		}
		cur = nil
	}
	for _, line := range strings.Split(src, "\n") {
		if strings.TrimSpace(line) == "# %%" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return cells
}

// StarterNotebook is the notebook `start` writes to /lab: how it is graded,
// the exercise's starter code, and a save cell per deliverable (nbformat 4.5).
func StarterNotebook(e *Exercise) []byte {
	type cell = map[string]any
	n := 0
	mk := func(kind, src string) cell {
		n++
		c := cell{"cell_type": kind, "id": fmt.Sprintf("kl-%d", n), "metadata": map[string]any{}, "source": src}
		if kind == "code" {
			c["execution_count"], c["outputs"] = nil, []any{}
		}
		return c
	}
	graded := "**How it's graded:** Check runs this notebook from top to bottom in a fresh kernel, on a copy of `/lab`"
	if len(e.Deliverables) > 0 {
		var files []string
		for _, d := range e.Deliverables {
			files = append(files, "`/lab/"+d+"`")
		}
		graded += " that doesn't have " + strings.Join(files, ", ") + " yet, so the notebook itself must write them. " +
			"Files you make by hand don't count."
	} else {
		graded += ", then calls the functions the task names on its own data."
	}
	cells := []cell{
		mk("markdown", fmt.Sprintf("# %s — %s\n\nThe task is on the exercise page and in `/lab/TASK.md`; its data is in `/lab`.\n\n%s\n\n"+
			"Save the notebook (ctrl+s) before you press Check, and use *Kernel → Restart Kernel and Run All Cells* to try it the way the check does.",
			e.ID, e.Title, graded)),
		mk("code", "import numpy as np\nimport pandas as pd\n\n%matplotlib inline"),
	}
	for _, c := range StarterCells(e.Starter) {
		cells = append(cells, mk("code", c))
	}
	cells = append(cells, mk("code", "# your analysis"))
	for _, it := range NotebookGuide(e) {
		cells = append(cells, mk("markdown", "## Save `/lab/"+it.File+"`\n\n"+it.How))
		if it.Code != "" {
			cells = append(cells, mk("code", it.Code))
		}
	}
	b, _ := json.MarshalIndent(map[string]any{
		"cells": cells,
		"metadata": map[string]any{
			"kernelspec":    map[string]any{"display_name": "Python 3 (ipykernel)", "language": "python", "name": "python3"},
			"language_info": map[string]any{"name": "python"},
		},
		"nbformat": 4, "nbformat_minor": 5,
	}, "", " ")
	return b
}
