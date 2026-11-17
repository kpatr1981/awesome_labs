package lab

import (
	"io"
	"os/exec"
	"strings"
	"testing"
)

func TestCommandConn(t *testing.T) {
	c, err := commandConn(exec.Command("cat"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(c, "hello\n"); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 6)
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "hello\n" {
		t.Fatalf("read %q, %v", buf, err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Write([]byte("x")); err == nil {
		t.Fatal("write after close succeeded")
	}
}

func TestJupyterScriptPinsAndFlags(t *testing.T) {
	for _, want := range []string{"--ServerApp.base_url=" + JupyterBase, "127.0.0.1:" + jupyterPort + ":8888", "--memory " + jupyterMemory, `kl.token=$TOKEN`, "-v /lab:/lab"} {
		if !strings.Contains(jupyterScript, want) {
			t.Errorf("start script lacks %q", want)
		}
	}
	if !strings.Contains(jupyterDockerfile, "FROM kl-datascience:1") {
		t.Error("the notebook image must build on the data-science image")
	}
}
