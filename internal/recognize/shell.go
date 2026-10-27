package recognize

import (
	"path/filepath"
	"strings"
)

// segment is one simple command inside a shell command line.
type segment struct {
	Env        map[string]string // leading NAME=value assignments
	Prog       string            // program basename ("aws")
	ProgPath   string            // program as written ("/usr/local/bin/aws", "./aws")
	Args       []string
	Background bool // ran with &, nohup, setsid, disown
	Opaque     bool // contains constructs we can't read (eval, $(...) as program, here-docs)
	Nested     bool // came from inside sh -c '…' (the outer shell and its environment also run)
}

// splitShell breaks a command line into simple commands. It understands
// quoting, the common separators (; && || | & newline), subshells and command
// substitutions (their contents become segments of their own). It is
// deliberately conservative: anything it can't follow marks the segment
// Opaque so the policy asks.
func splitShell(cmd string) []segment {
	var segs []segment
	var words []string
	var cur strings.Builder
	inWord := false
	opaque := false
	background := false

	flushWord := func() {
		if inWord {
			words = append(words, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	flushSeg := func(bg bool) {
		flushWord()
		if len(words) > 0 {
			segs = append(segs, buildSegment(words, opaque, background || bg))
		}
		words = nil
		opaque = false
		background = false
	}

	rs := []rune(cmd)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\\' && i+1 < len(rs):
			if rs[i+1] == '\n' { // line continuation
				i++
				continue
			}
			cur.WriteRune(rs[i+1])
			inWord = true
			i++
		case c == '\'':
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				j++
			}
			cur.WriteString(string(rs[i+1 : min(j, len(rs))]))
			inWord = true
			i = j
		case c == '"':
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
				} else if rs[j] == '$' && j+1 < len(rs) && rs[j+1] == '(' {
					inner, end := matchParen(rs, j+1)
					segs = append(segs, splitShell(inner)...)
					cur.WriteString("$(" + inner + ")")
					j = end
				} else if rs[j] == '`' {
					k := j + 1
					for k < len(rs) && rs[k] != '`' {
						k++
					}
					segs = append(segs, splitShell(string(rs[j+1:min(k, len(rs))]))...)
					j = k
				}
				if j < len(rs) {
					cur.WriteRune(rs[j])
				}
				j++
			}
			// drop the closing quote we copied
			s := cur.String()
			if strings.HasSuffix(s, `"`) {
				cur.Reset()
				cur.WriteString(strings.TrimSuffix(s, `"`))
			}
			inWord = true
			i = j
		case c == '$' && i+1 < len(rs) && rs[i+1] == '(':
			inner, end := matchParen(rs, i+1)
			segs = append(segs, splitShell(inner)...)
			if !inWord && len(words) == 0 {
				opaque = true // the program name comes from a substitution
			}
			cur.WriteString("$(" + inner + ")")
			inWord = true
			i = end
		case c == '`':
			j := i + 1
			for j < len(rs) && rs[j] != '`' {
				j++
			}
			segs = append(segs, splitShell(string(rs[i+1:min(j, len(rs))]))...)
			if !inWord && len(words) == 0 {
				opaque = true
			}
			inWord = true
			i = j
		case c == '(' && !inWord && len(words) == 0:
			inner, end := matchParen(rs, i)
			segs = append(segs, splitShell(inner)...)
			i = end
		case c == '<' && i+1 < len(rs) && rs[i+1] == '<':
			opaque = true // here-doc: the body is input we don't read
			cur.WriteRune(c)
			inWord = true
		case c == ';' || c == '\n':
			flushSeg(false)
		case c == '&':
			if i+1 < len(rs) && rs[i+1] == '&' {
				flushSeg(false)
				i++
			} else if i+1 < len(rs) && rs[i+1] == '>' { // &> redirect
				cur.WriteRune(c)
				inWord = true
			} else if i > 0 && (rs[i-1] == '>' || rs[i-1] == '<') { // 2>&1
				cur.WriteRune(c)
				inWord = true
			} else {
				flushSeg(true)
			}
		case c == '|':
			if i+1 < len(rs) && rs[i+1] == '|' {
				i++
			}
			flushSeg(false)
		case c == ' ' || c == '\t':
			flushWord()
		case c == '#' && !inWord:
			for i < len(rs) && rs[i] != '\n' {
				i++
			}
			i--
		default:
			cur.WriteRune(c)
			inWord = true
		}
	}
	flushSeg(false)
	return segs
}

// matchParen returns the text inside the parenthesis that opens at rs[open]
// and the index of its closing parenthesis.
func matchParen(rs []rune, open int) (string, int) {
	depth := 0
	for j := open; j < len(rs); j++ {
		switch rs[j] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return string(rs[open+1 : j]), j
			}
		case '\'':
			for j++; j < len(rs) && rs[j] != '\''; j++ {
			}
		}
	}
	return string(rs[open+1:]), len(rs) - 1
}

var wrappers = map[string]bool{"sudo": true, "env": true, "time": true, "command": true, "builtin": true,
	"exec": true, "nice": true, "ionice": true, "stdbuf": true, "timeout": true, "caffeinate": true}
var detachers = map[string]bool{"nohup": true, "setsid": true, "disown": true, "daemonize": true, "screen": true, "tmux": true}

func buildSegment(words []string, opaque, bg bool) segment {
	s := segment{Env: map[string]string{}, Opaque: opaque, Background: bg}
	i := 0
	for i < len(words) {
		w := words[i]
		if eq := strings.IndexByte(w, '='); eq > 0 && isEnvName(w[:eq]) && s.Prog == "" {
			s.Env[w[:eq]] = w[eq+1:]
			i++
			continue
		}
		base := filepath.Base(w)
		// A wrapper given by path counts only if it's a system copy; a
		// project file named "env" or "nohup" is the program itself.
		if (detachers[base] || wrappers[base]) && strings.Contains(w, "/") && !systemPath(w) {
			break
		}
		if detachers[base] {
			s.Background = true
			i++
			continue
		}
		if wrappers[base] {
			i++
			// skip the wrapper's own options and arguments (timeout 10, nice -n 5, env -i)
			for i < len(words) && (strings.HasPrefix(words[i], "-") || isNumberish(words[i])) {
				i++
			}
			continue
		}
		if base == "eval" || base == "source" || base == "." || base == "xargs" || base == "parallel" || base == "watch" {
			s.Opaque = true
		}
		break
	}
	if i < len(words) {
		s.ProgPath = words[i]
		s.Prog = filepath.Base(words[i])
		s.Args = words[i+1:]
	}
	if strings.ContainsAny(s.ProgPath, "$`") {
		s.Opaque = true
	}
	return s
}

// Expand flattens `sh -c "a && b"` style wrappers into their inner segments.
func expand(segs []segment) []segment {
	var out []segment
	for _, s := range segs {
		if isShell(s.Prog) && (!strings.Contains(s.ProgPath, "/") || systemPath(s.ProgPath)) {
			for j, a := range s.Args {
				if (a == "-c" || a == "-lc" || a == "-ic" || a == "-cl") && j+1 < len(s.Args) {
					inner := expand(splitShell(s.Args[j+1]))
					for k := range inner {
						inner[k].Background = inner[k].Background || s.Background
						inner[k].Nested = true
						for ek, ev := range s.Env { // the outer shell's environment applies too
							if _, ok := inner[k].Env[ek]; !ok {
								inner[k].Env[ek] = ev
							}
						}
					}
					out = append(out, inner...)
					goto next
				}
			}
		}
		out = append(out, s)
	next:
	}
	return out
}

// systemPath reports whether an explicit program path is an installed system
// copy rather than something in the project.
func systemPath(p string) bool {
	for _, d := range []string{"/bin/", "/usr/bin/", "/usr/sbin/", "/sbin/", "/usr/local/bin/", "/opt/homebrew/bin/"} {
		if strings.HasPrefix(p, d) && !strings.Contains(p[len(d):], "/") {
			return true
		}
	}
	return false
}

func isShell(p string) bool {
	switch p {
	case "sh", "bash", "zsh", "dash", "ksh", "fish":
		return true
	}
	return false
}

func isEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if !(c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (i > 0 && c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func isNumberish(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9') && c != '.' && c != 's' && c != 'm' {
			return false
		}
	}
	return true
}

// flagValue returns the value of --name / --name=value / -n in args.
func flagValue(args []string, names ...string) (string, bool) {
	for i, a := range args {
		for _, n := range names {
			if a == n && i+1 < len(args) {
				return args[i+1], true
			}
			if strings.HasPrefix(a, n+"=") {
				return a[len(n)+1:], true
			}
		}
	}
	return "", false
}

// flagValues returns every value given for a flag (curl uses the last one).
func flagValues(args []string, names ...string) []string {
	var out []string
	for i, a := range args {
		for _, n := range names {
			if a == n && i+1 < len(args) {
				out = append(out, args[i+1])
			} else if strings.HasPrefix(a, n+"=") {
				out = append(out, a[len(n)+1:])
			}
		}
	}
	return out
}

// curlShortWithValue are curl's one-letter options that take a value; it
// accepts them attached (-XDELETE) or combined after other letters (-sXPOST).
const curlShortWithValue = "XdHuKTFoeAbcEmrwxyYzCDUPQt"

// normalizeCurl splits curl's attached and combined short options so the
// rest of the recognizer sees "-X", "DELETE" (release review).
func normalizeCurl(args []string) []string {
	var out []string
	for _, a := range args {
		if len(a) > 2 && a[0] == '-' && a[1] != '-' {
			for i := 1; i < len(a); i++ {
				c := a[i]
				out = append(out, "-"+string(c))
				if strings.IndexByte(curlShortWithValue, c) >= 0 {
					if i+1 < len(a) {
						out = append(out, a[i+1:])
					}
					break
				}
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func hasFlag(args []string, names ...string) bool {
	for _, a := range args {
		for _, n := range names {
			if a == n || strings.HasPrefix(a, n+"=") {
				return true
			}
		}
	}
	return false
}

// positional returns args that are not flags or flag values (best effort:
// a flag followed by a non-flag word is assumed to take it as a value only
// when the flag is in valueFlags).
func positional(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if !strings.Contains(a, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") && takesValue(a) {
				i++
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

var boolFlags = map[string]bool{"--force": true, "-f": true, "--recursive": true, "-r": true, "--quiet": true, "-q": true,
	"--yes": true, "-y": true, "--live": true, "--dryrun": true, "--dry-run": true, "--no-cli-pager": true, "--debug": true,
	"--all": true, "-a": true, "--json": true, "--verbose": true, "-v": true, "--help": true, "-h": true, "--delete": true,
	"--prod": true, "--production": true, "--human-readable": true, "--summarize": true, "-s": true, "-S": true, "-L": true,
	"-k": true, "-i": true, "-I": true, "--silent": true, "--fail": true, "--no-verify-ssl": true, "--async": true, "--wait": true}

func takesValue(flag string) bool { return !boolFlags[flag] }
