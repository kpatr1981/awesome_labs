package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Setting is one top-level scalar setting to write.
type Setting struct {
	Key   string
	Value any // string, bool or int
}

// Set writes settings into dir/config.yaml, keeping everything else in the
// file as it is: comments, VM sizes, unknown keys. A key already set has its
// value replaced on its own line (keeping a trailing comment); a key that only
// appears commented out ("# on_exit: stop", as the template has it) is
// uncommented there; any other key is appended. The result must load and
// validate, or nothing is written.
func Set(dir string, settings ...Setting) (Config, error) {
	p := Path(dir)
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		b, err = []byte(template), nil
	}
	if err != nil {
		return Config{}, err
	}
	text := string(b)
	for _, st := range settings {
		v, err := scalar(st.Value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", st.Key, err)
		}
		text = setKey(text, st.Key, v)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		return Config{}, err
	}
	// load the candidate through the normal path, so what is written is what
	// the next start reads
	c, err := loadFile(tmp)
	if err != nil {
		os.Remove(tmp)
		return Config{}, err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return Config{}, err
	}
	return c, nil
}

func scalar(v any) (string, error) {
	switch x := v.(type) {
	case string:
		if x == "" || strings.ContainsAny(x, ":#\n\"'{}[],&*!|>%@`") {
			return strconv.Quote(x), nil
		}
		return x, nil
	case bool:
		return strconv.FormatBool(x), nil
	case int:
		return strconv.Itoa(x), nil
	}
	return "", fmt.Errorf("unsupported value %T", v)
}

func setKey(text, key, value string) string {
	k := regexp.QuoteMeta(key)
	// set: "key: old   # comment" (top level, not indented)
	set := regexp.MustCompile(`(?m)^` + k + `:[ \t]*([^#\n]*?)([ \t]+#[^\n]*)?$`)
	if loc := set.FindStringSubmatchIndex(text); loc != nil {
		comment := ""
		if loc[4] >= 0 {
			comment = text[loc[4]:loc[5]]
		}
		return text[:loc[0]] + key + ": " + value + comment + text[loc[1]:]
	}
	// commented out: "# key: default"
	off := regexp.MustCompile(`(?m)^#[ \t]*` + k + `:[^\n]*$`)
	if loc := off.FindStringIndex(text); loc != nil {
		return text[:loc[0]] + key + ": " + value + text[loc[1]:]
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + key + ": " + value + "\n"
}
