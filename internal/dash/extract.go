package dash

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Parse turns a command's output into a value: JSON values, a number, rows
// (CSV, TSV), lines or plain text, depending on the source's format.
func Parse(output, format string) (any, error) {
	text := strings.TrimSpace(output)
	switch format {
	case "", "auto":
		if text == "" {
			return nil, fmt.Errorf("the command printed nothing")
		}
		if v, err := parseJSON(text); err == nil {
			return v, nil
		}
		if n, ok := ParseNumber(text); ok {
			return n, nil
		}
		return text, nil
	case "json":
		v, err := parseJSON(text)
		if err != nil {
			return nil, fmt.Errorf("the output isn't JSON: %v", err)
		}
		return v, nil
	case "number":
		n, ok := ParseNumber(text)
		if !ok {
			return nil, fmt.Errorf("the output isn't a number: %q", short(text, 60))
		}
		return n, nil
	case "csv", "tsv":
		return parseTable(text, format == "tsv")
	case "lines":
		var out []any
		for _, l := range strings.Split(text, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				out = append(out, l)
			}
		}
		return out, nil
	case "text":
		return text, nil
	}
	return nil, fmt.Errorf("unknown format %q", format)
}

func parseJSON(text string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		// several JSON documents, one per line (JSON Lines): a list
		out := []any{normalize(v)}
		for dec.More() {
			var next any
			if err := dec.Decode(&next); err != nil {
				return nil, err
			}
			out = append(out, normalize(next))
		}
		return out, nil
	}
	return normalize(v), nil
}

// normalize turns json.Number into float64 all the way down.
func normalize(v any) any {
	switch t := v.(type) {
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	case []any:
		for i := range t {
			t[i] = normalize(t[i])
		}
		return t
	case map[string]any:
		for k := range t {
			t[k] = normalize(t[k])
		}
		return t
	}
	return v
}

func parseTable(text string, tabs bool) (any, error) {
	r := csv.NewReader(strings.NewReader(text))
	if tabs {
		r.Comma = '\t'
		r.LazyQuotes = true
	}
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("the output isn't a table: %v", err)
	}
	if len(recs) == 0 {
		return []any{}, nil
	}
	head := recs[0]
	rows := []any{}
	for _, rec := range recs[1:] {
		row := map[string]any{}
		for i, h := range head {
			if i < len(rec) {
				row[h] = cell(rec[i])
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func cell(s string) any {
	if n, ok := ParseNumber(s); ok {
		return n
	}
	return s
}

// ParseNumber reads "1234", "1,234", "12.5", "$12", "45%" and " 7 ". It
// refuses anything with other text in it.
func ParseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	for _, p := range []string{"$", "€", "£", "US$"} {
		s = strings.TrimPrefix(s, p)
	}
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// checkPath accepts paths like "data.0.count", "data[0].count",
// "$.items.length", "rows.last.total".
func checkPath(path string) error {
	_, err := pathParts(path)
	return err
}

func pathParts(path string) ([]string, error) {
	p := strings.TrimSpace(path)
	p = strings.TrimPrefix(p, "$")
	p = strings.TrimPrefix(p, ".")
	if p == "" {
		return nil, nil
	}
	p = strings.ReplaceAll(p, "[", ".")
	p = strings.ReplaceAll(p, "]", "")
	var out []string
	for _, part := range strings.Split(p, ".") {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part == "" {
			return nil, fmt.Errorf("it has an empty part")
		}
		out = append(out, part)
	}
	if len(out) > 12 {
		return nil, fmt.Errorf("it is too deep")
	}
	return out, nil
}

// Lookup follows a path into a parsed value. The last part may be length
// (or count), first or last.
func Lookup(v any, path string) (any, error) {
	parts, err := pathParts(path)
	if err != nil {
		return nil, err
	}
	cur := v
	for i, part := range parts {
		switch t := cur.(type) {
		case map[string]any:
			if next, ok := t[part]; ok {
				cur = next
				continue
			}
			if part == "length" || part == "count" {
				cur = float64(len(t))
				continue
			}
			return nil, fmt.Errorf("%q isn't in the output (it has %s)", strings.Join(parts[:i+1], "."), keysOf(t))
		case []any:
			switch part {
			case "length", "count":
				cur = float64(len(t))
				continue
			case "first":
				part = "0"
			case "last":
				part = strconv.Itoa(len(t) - 1)
			}
			n, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("%q: the output has a list there, so use a number, length, first or last", strings.Join(parts[:i+1], "."))
			}
			if n < 0 {
				n += len(t)
			}
			if n < 0 || n >= len(t) {
				return nil, fmt.Errorf("%q: the list has %d items", strings.Join(parts[:i+1], "."), len(t))
			}
			cur = t[n]
		case string:
			if part == "length" || part == "count" {
				cur = float64(len([]rune(t)))
				continue
			}
			return nil, fmt.Errorf("%q: the output has text there, not an object", strings.Join(parts[:i+1], "."))
		default:
			return nil, fmt.Errorf("%q: there is nothing to look into there", strings.Join(parts[:i+1], "."))
		}
	}
	return cur, nil
}

func keysOf(m map[string]any) string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	if len(ks) > 8 {
		ks = append(ks[:8], "…")
	}
	if len(ks) == 0 {
		return "nothing"
	}
	return strings.Join(ks, ", ")
}

// Number reads a value as a number: numbers, numeric text, true/false, and
// one-value objects or lists (a single SQL row like [{"count": 12}]).
func Number(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case string:
		return ParseNumber(t)
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	case []any:
		if len(t) == 1 {
			return Number(t[0])
		}
	case map[string]any:
		if len(t) == 1 {
			for _, x := range t {
				return Number(x)
			}
		}
	}
	return 0, false
}

// Text writes a value for a text tile.
func Text(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case []any:
		allText := true
		for _, x := range t {
			if _, ok := x.(string); !ok {
				allText = false
			}
		}
		if allText {
			var lines []string
			for _, x := range t {
				lines = append(lines, x.(string))
			}
			return strings.Join(lines, "\n")
		}
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSpace(b.String())
}

// Rows reads a value as table rows: a list of objects, a list of lists whose
// first row is the header, a list of plain values, or one object (one row
// per key).
func Rows(v any) ([]map[string]any, []string, error) {
	switch t := v.(type) {
	case []any:
		if len(t) == 0 {
			return []map[string]any{}, nil, nil
		}
		if _, ok := t[0].(map[string]any); ok {
			var cols []string
			seen := map[string]bool{}
			rows := make([]map[string]any, 0, len(t))
			for _, x := range t {
				m, ok := x.(map[string]any)
				if !ok {
					return nil, nil, fmt.Errorf("the list mixes objects and other values")
				}
				// keep the first row's order, then add new keys sorted
				keys := make([]string, 0, len(m))
				for k := range m {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					if !seen[k] {
						seen[k] = true
						cols = append(cols, k)
					}
				}
				rows = append(rows, m)
			}
			return rows, cols, nil
		}
		if head, ok := t[0].([]any); ok {
			cols := make([]string, len(head))
			for i, h := range head {
				cols[i] = Text(h)
			}
			rows := []map[string]any{}
			for _, x := range t[1:] {
				rec, ok := x.([]any)
				if !ok {
					return nil, nil, fmt.Errorf("the list mixes lists and other values")
				}
				row := map[string]any{}
				for i, c := range cols {
					if i < len(rec) {
						row[c] = rec[i]
					}
				}
				rows = append(rows, row)
			}
			return rows, cols, nil
		}
		rows := make([]map[string]any, 0, len(t))
		for _, x := range t {
			rows = append(rows, map[string]any{"value": x})
		}
		return rows, []string{"value"}, nil
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		rows := make([]map[string]any, 0, len(keys))
		for _, k := range keys {
			rows = append(rows, map[string]any{"name": k, "value": t[k]})
		}
		return rows, []string{"name", "value"}, nil
	}
	return nil, nil, fmt.Errorf("the output isn't a list or an object")
}

// Bar is one bar of a bars tile, or one point of a line from rows.
type Bar struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

// Bars reads label/value pairs from rows, or from an object of numbers.
func Bars(v any, labelField, valueField string) ([]Bar, error) {
	if m, ok := v.(map[string]any); ok && labelField == "" && valueField == "" {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var out []Bar
		for _, k := range keys {
			if n, ok := Number(m[k]); ok {
				out = append(out, Bar{Label: k, Value: n})
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("the object has no numbers")
		}
		return out, nil
	}
	rows, cols, err := Rows(v)
	if err != nil {
		return nil, err
	}
	if labelField == "" || valueField == "" {
		// guess: the first text column and the first number column
		for _, c := range cols {
			if len(rows) == 0 {
				break
			}
			_, isNum := rows[0][c].(float64)
			if valueField == "" && isNum && c != labelField {
				valueField = c
				continue
			}
			if labelField == "" && c != valueField {
				labelField = c
			}
		}
	}
	if labelField == "" || valueField == "" {
		return nil, fmt.Errorf("say which fields hold the label and the value (labelField, valueField)")
	}
	out := make([]Bar, 0, len(rows))
	for i, r := range rows {
		n, ok := Number(r[valueField])
		if !ok {
			return nil, fmt.Errorf("row %d: %q isn't a number", i+1, valueField)
		}
		out = append(out, Bar{Label: Text(r[labelField]), Value: n})
	}
	return out, nil
}

func short(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
