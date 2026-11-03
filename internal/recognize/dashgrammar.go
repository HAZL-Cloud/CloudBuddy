package recognize

import (
	"path/filepath"
	"strings"
)

// Dashboard commands run with nobody watching, so CloudBuddy parses them
// itself with a small grammar instead of guessing at shell (D-067; three
// Codex review rounds kept finding shell syntax the guesses misread: zsh
// glob qualifiers, `>&1.txt`, `env -S`, options hidden in variables).
//
// The grammar: words made of plain characters, 'single-quoted' and
// "double-quoted" text, $NAME variables, the separators | && || ;, and
// only the redirections 2>/dev/null, >/dev/null, 2>&1 and >&2. A command
// outside it can't be a dashboard command at all: put the steps in a
// script file, which CloudBuddy can hash and the user can approve.

// DashPart is a piece of a word: literal text, or a variable the shell expands.
type DashPart struct {
	Lit string
	Var string
}

// DashWord is one word of a dashboard command.
type DashWord struct {
	Parts []DashPart
}

// Text is the word with its variables written as $NAME.
func (w DashWord) Text() string {
	var b strings.Builder
	for _, p := range w.Parts {
		if p.Var != "" {
			b.WriteString("$" + p.Var)
		} else {
			b.WriteString(p.Lit)
		}
	}
	return b.String()
}

// HasVars reports whether the shell expands something in the word.
func (w DashWord) HasVars() bool {
	for _, p := range w.Parts {
		if p.Var != "" {
			return true
		}
	}
	return false
}

// Expand is the word as the shell will pass it (zsh doesn't split or glob
// the value of a variable).
func (w DashWord) Expand(env map[string]string) string {
	var b strings.Builder
	for _, p := range w.Parts {
		if p.Var != "" {
			b.WriteString(env[p.Var])
		} else {
			b.WriteString(p.Lit)
		}
	}
	return b.String()
}

// DashSegment is one simple command: NAME=value assignments, the program and
// its arguments.
type DashSegment struct {
	Env   []DashWord // each NAME=value, as written
	Words []DashWord // program first
	Raw   string     // the segment's text in the command
}

// Prog is the program as written.
func (s DashSegment) Prog() string {
	if len(s.Words) == 0 {
		return ""
	}
	return s.Words[0].Text()
}

// Args are the arguments with variables written as $NAME.
func (s DashSegment) Args() []string {
	var out []string
	for _, w := range s.Words[1:] {
		out = append(out, w.Text())
	}
	return out
}

func plainChar(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("_./:=,@%+^-", c)
}

func nameChar(c rune, first bool) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (!first && c >= '0' && c <= '9')
}

// redirects are the only redirections a dashboard command may use, written
// right after an optional 1 or 2.
var redirects = []string{">/dev/null", ">&1", ">&2"}

// ParseDashCommand splits a command by the dashboard grammar. It returns why
// not when the command is outside it.
func ParseDashCommand(cmd string) ([]DashSegment, string) {
	rs := []rune(cmd)
	var segs []DashSegment
	var words []DashWord
	var parts []DashPart
	var lit strings.Builder
	inWord := false
	segStart := 0

	flushLit := func() {
		if lit.Len() > 0 {
			parts = append(parts, DashPart{Lit: lit.String()})
			lit.Reset()
		}
	}
	flushWord := func() {
		flushLit()
		if inWord {
			words = append(words, DashWord{Parts: parts})
		}
		parts, inWord = nil, false
	}
	endSeg := func(at int, sep bool) string {
		flushWord()
		raw := strings.TrimSpace(string(rs[segStart:at]))
		if len(words) == 0 {
			if sep || len(segs) > 0 {
				return "it has an empty command around a separator"
			}
			return ""
		}
		seg := DashSegment{Raw: raw}
		i := 0
		for ; i < len(words); i++ {
			w := words[i]
			if len(w.Parts) == 0 || w.Parts[0].Var != "" {
				break
			}
			name, _, ok := strings.Cut(w.Parts[0].Lit, "=")
			if !ok || name == "" || strings.ContainsAny(name, "./:,@%+^-") || !nameChar(rune(name[0]), true) {
				break
			}
			seg.Env = append(seg.Env, w)
		}
		seg.Words = words[i:]
		if len(seg.Words) == 0 {
			return "it only sets variables"
		}
		if seg.Words[0].HasVars() {
			return "the program's name comes from a variable"
		}
		segs = append(segs, seg)
		words = nil
		segStart = at + 1
		return ""
	}
	readVar := func(i int) (string, int) { // rs[i] == '$'
		j := i + 1
		for j < len(rs) && nameChar(rs[j], j == i+1) {
			j++
		}
		return string(rs[i+1 : j]), j
	}

	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == ' ' || c == '\t':
			flushWord()
		case c == '\'':
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				j++
			}
			if j >= len(rs) {
				return nil, "a quote isn't closed"
			}
			lit.WriteString(string(rs[i+1 : j]))
			inWord, i = true, j
		case c == '"':
			j := i + 1
			for ; j < len(rs) && rs[j] != '"'; j++ {
				switch rs[j] {
				case '\\', '`':
					return nil, "it has " + string(rs[j]) + " inside double quotes"
				case '$':
					name, end := readVar(j)
					if name == "" {
						return nil, "it has a $ that isn't a plain $NAME variable"
					}
					flushLit()
					parts = append(parts, DashPart{Var: name})
					j = end - 1
				default:
					lit.WriteRune(rs[j])
				}
			}
			if j >= len(rs) {
				return nil, "a quote isn't closed"
			}
			inWord, i = true, j
		case c == '$':
			name, end := readVar(i)
			if name == "" {
				return nil, "it has a $ that isn't a plain $NAME variable"
			}
			flushLit()
			parts = append(parts, DashPart{Var: name})
			inWord, i = true, end-1
		case c == '|' || c == ';' || (c == '&' && i+1 < len(rs) && rs[i+1] == '&'):
			at := i
			if (c == '|' && i+1 < len(rs) && rs[i+1] == '|') || c == '&' {
				i++
			}
			if why := endSeg(at, true); why != "" {
				return nil, why
			}
			segStart = i + 1
		case c == '>':
			// a redirection: only the harmless ones, as a whole word, after an optional 1 or 2
			flushLit()
			fd := ""
			if inWord && len(parts) == 1 && parts[0].Var == "" && (parts[0].Lit == "1" || parts[0].Lit == "2") {
				fd = parts[0].Lit
				parts, inWord = nil, false
			}
			if inWord {
				return nil, "it has a > CloudBuddy can't read"
			}
			rest := string(rs[i:])
			ok := false
			for _, r := range redirects {
				if strings.HasPrefix(rest, r) {
					end := i + len([]rune(r))
					if end == len(rs) || rs[end] == ' ' || rs[end] == '\t' || rs[end] == '|' || rs[end] == ';' || rs[end] == '&' {
						if r == ">/dev/null" || fd == "2" || (fd == "1" && r == ">&2") || (fd == "" && r == ">&2") {
							ok = true
							i = end - 1
						}
					}
					break
				}
			}
			if !ok {
				return nil, "it redirects output somewhere other than 2>/dev/null, 2>&1 or >&2"
			}
		case plainChar(c):
			lit.WriteRune(c)
			inWord = true
		case c == '\n' || c == '\r':
			return nil, "it has several lines"
		default:
			return nil, "it uses " + string(c) + " outside quotes"
		}
	}
	if why := endSeg(len(rs), false); why != "" {
		return nil, why
	}
	if len(segs) == 0 {
		return nil, "it is empty"
	}
	return segs, ""
}

// StrictRead reports whether every piece of a dashboard command (already
// inside the grammar) is a read CloudBuddy recognizes: a cloud CLI read, a
// text tool with safe options, or one read-only database query. It returns
// "psql" or "sqlite3" for a database read, and why not otherwise.
func StrictRead(segs []DashSegment, env Env, ads *Adapters) (string, string) {
	engine := ""
	for _, s := range segs {
		prog := s.Prog()
		if strings.Contains(prog, "/") {
			return "", "it runs " + prog
		}
		hasVars := false
		for _, w := range s.Words[1:] {
			if w.HasVars() {
				hasVars = true
			}
		}
		switch {
		case prog == "psql":
			for _, e := range s.Env {
				name, _, _ := strings.Cut(e.Text(), "=")
				if !pgEnvAllowed[name] {
					return "", "it sets " + name
				}
			}
			if why := psqlWords(s.Words[1:], env.AgentEnv); why != "" {
				return "", why
			}
			engine = "psql"
		case prog == "sqlite3":
			if len(s.Env) > 0 || hasVars {
				return "", "it uses variables"
			}
			if why := sqliteRead(s.Args()); why != "" {
				return "", why
			}
			engine = "sqlite3"
		case dashLocalTools[prog]:
			if len(s.Env) > 0 || hasVars {
				return "", "it gives " + prog + " a variable, which could hold an option"
			}
			if !textToolSafe(segment{Prog: prog, ProgPath: prog, Args: s.Args(), Env: map[string]string{}}) {
				return "", prog + " is used with options that write or show secrets"
			}
		case cloudCLIs[prog].Provider != "" && !cloudCLIs[prog].RunsCode:
			if len(s.Env) > 0 || hasVars {
				return "", "it gives " + prog + " a variable, which could hold an option"
			}
			d := Recognize(Call{Agent: "claude", Via: "shell", Command: s.Raw, WantsNetwork: true, Cwd: env.Workdir}, env, ads)
			bad := d.Action != "read" || !d.ReviewedRead || d.Opaque || d.Network != "A" || len(d.ExternalFiles) > 0 || len(d.Protected) > 0
			for _, t := range d.Tags {
				if t != "live" {
					bad = true
				}
			}
			if bad {
				return "", "CloudBuddy doesn't know that “" + oneLine(s.Raw) + "” only reads"
			}
		default:
			return "", "it runs " + filepath.Base(prog)
		}
	}
	return engine, ""
}

// psqlWords checks a psql read: the options of psqlRead, -X required, and
// variables only in the connection arguments ("$DATABASE_URL").
func psqlWords(words []DashWord, environ map[string]string) string {
	args := make([]string, len(words))
	for i, w := range words {
		args[i] = w.Text()
		// a connection URL in a variable can carry options that turn the
		// read-only session off: check what it holds now
		if w.HasVars() {
			lo := strings.ToLower(w.Expand(environ))
			if strings.Contains(lo, "options") || strings.Contains(lo, "transaction") || strings.Contains(lo, "read_only") || strings.Contains(lo, "service") {
				return "the connection in " + w.Text() + " changes session settings"
			}
		}
	}
	noRC := false
	for i := 0; i < len(words); i++ {
		a := args[i]
		if words[i].HasVars() {
			// a variable is a value: it must be a connection argument, not an option or the query
			if strings.HasPrefix(a, "-") {
				return "a psql option comes from a variable"
			}
			if i > 0 && optionTakesValue(args[i-1]) {
				return "a psql option's value comes from a variable"
			}
		}
		if a == "-X" || a == "--no-psqlrc" || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "X")) {
			noRC = true
		}
	}
	if !noRC {
		return "psql must run with -X, so no psqlrc file runs before the query"
	}
	return psqlRead(args)
}

// optionTakesValue reports whether a psql option word expects the next word
// as its value (-c, -tAc, --host…).
func optionTakesValue(a string) bool {
	if strings.HasPrefix(a, "--") {
		takes, ok := psqlLong[a]
		return ok && takes
	}
	if strings.HasPrefix(a, "-") && len(a) > 1 {
		last := a[len(a)-1]
		takes, ok := psqlShort[last]
		// a value letter at the end, with nothing attached after it
		return ok && takes
	}
	return false
}

// SQLWrite finds a word that changes data in any psql or sqlite3 query of
// a command, however it is written (-cDELETE, -tAc, --command=). Commands
// outside the dashboard grammar are checked too: this decides "never".
func SQLWrite(command string) string {
	for _, s := range expand(splitShell(command)) {
		var queries []string
		switch s.Prog {
		case "psql":
			for i := 0; i < len(s.Args); i++ {
				a := s.Args[i]
				switch {
				case strings.HasPrefix(a, "--command="):
					queries = append(queries, strings.TrimPrefix(a, "--command="))
				case a == "--command" && i+1 < len(s.Args):
					queries = append(queries, s.Args[i+1])
				case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--"):
					if j := strings.IndexByte(a, 'c'); j > 0 {
						if rest := a[j+1:]; rest != "" {
							queries = append(queries, rest)
						} else if i+1 < len(s.Args) {
							queries = append(queries, s.Args[i+1])
						}
					}
				}
			}
		case "sqlite3":
			queries = append(queries, positional(s.Args)...)
		default:
			continue
		}
		for _, a := range s.Args { // anything else that looks like SQL
			if strings.ContainsAny(a, " \n") {
				queries = append(queries, a)
			}
		}
		for _, q := range queries {
			if w := sqlWriteWord(q); w != "" {
				return w
			}
		}
	}
	return ""
}
