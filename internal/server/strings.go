package server

// The page texts live in web/i18n/<lang>.json, one file per language, English
// the reference (owner's choice 2026-10-28: English is the default). The
// languages are the ones the ZimaOS UI ships (assets/<lang>-*.js on .143,
// measured 2026-10-28). /strings.js hands all of them to both pages as one
// script: the TV runs Chrome 53, so no modules and no fetch before the first
// paint.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"
)

// buildStrings reads every web/i18n/*.json into "var I18N_STRINGS = {...};".
func buildStrings() ([]byte, error) {
	files, err := fs.Glob(webFS, "web/i18n/*.json")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	all := map[string]map[string]string{}
	for _, f := range files {
		b, err := webFS.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var d map[string]string
		if err := json.Unmarshal(b, &d); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		all[strings.TrimSuffix(path.Base(f), ".json")] = d
	}
	if _, ok := all["en"]; !ok {
		return nil, fmt.Errorf("web/i18n/en.json missing")
	}
	b, err := json.Marshal(all)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.WriteString("var I18N_STRINGS = ")
	out.Write(b)
	out.WriteString(";\n")
	return out.Bytes(), nil
}

func (s *Server) stringsJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(s.strings)
}
