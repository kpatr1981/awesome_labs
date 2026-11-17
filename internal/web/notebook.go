package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"awesome_labs/internal/exercise"
	"awesome_labs/internal/lab"
)

// The notebook guide and starter notebook come from the exercise package;
// `start` writes the starter notebook. ensureStarter puts it back when it's
// gone (deleted, or a lab started by an older build) as JupyterLab opens.

// ensureStarter saves the exercise's starter notebook through JupyterLab's
// contents API, unless a file of that name is already there (yours).
func (s *server) ensureStarter(ctx context.Context, token string, ex *exercise.Exercise, log io.Writer) error {
	c := &http.Client{Transport: s.jupyter.proxy.Transport, Timeout: 20 * time.Second}
	url := "http://jupyter" + lab.JupyterBase + "api/contents/" + ex.NotebookName()
	do := func(method string, body []byte) (int, error) {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return 0, err
		}
		req.Header.Set("Authorization", "token "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := c.Do(req)
		if err != nil {
			return 0, err
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		return res.StatusCode, nil
	}
	code, err := do(http.MethodGet, nil)
	switch {
	case err != nil:
		return err
	case code == http.StatusOK:
		fmt.Fprintf(log, "==> opening /lab/%s\n", ex.NotebookName())
		return nil
	case code != http.StatusNotFound:
		return fmt.Errorf("looking for /lab/%s: HTTP %d", ex.NotebookName(), code)
	}
	body, _ := json.Marshal(map[string]any{"type": "notebook", "format": "json", "content": json.RawMessage(exercise.StarterNotebook(ex))})
	if code, err = do(http.MethodPut, body); err != nil {
		return err
	} else if code != http.StatusOK && code != http.StatusCreated {
		return fmt.Errorf("saving /lab/%s: HTTP %d", ex.NotebookName(), code)
	}
	fmt.Fprintf(log, "==> /lab/%s was missing: wrote a fresh one\n", ex.NotebookName())
	return nil
}
