package recognize

import (
	"strings"
)

// Helpers for dashboard commands (D-067, D-068), which run on a timer with
// nobody to ask. They are stricter than the chat gate: anything they can't
// read asks the user once.

// dashTextTools may follow a database query in a pipe: they only shape text.
// (safeTools also has tee, mkdir and touch, which write files.)
var dashTextTools = map[string]bool{"echo": true, "printf": true, "jq": true, "yq": true, "grep": true, "egrep": true,
	"fgrep": true, "head": true, "tail": true, "wc": true, "uniq": true, "cut": true, "tr": true, "column": true, "cat": true,
	"base64": true, "true": true}

// pgEnvAllowed are the libpq variables a psql read may set in the command.
// PGOPTIONS isn't one of them: CloudBuddy sets it to keep the session
// read-only.
var pgEnvAllowed = map[string]bool{"PGHOST": true, "PGPORT": true, "PGUSER": true, "PGDATABASE": true, "PGPASSWORD": true,
	"PGSSLMODE": true, "PGCONNECT_TIMEOUT": true, "PGAPPNAME": true, "TZ": true, "LANG": true, "LC_ALL": true}

// SQLCheck is what SQLRead found.
type SQLCheck struct {
	Engine string // psql | sqlite3 when the command is one read-only query
	Why    string // why it isn't, when it is psql or sqlite3
	// Write is a word that only appears in queries that change data
	// (DELETE, UPDATE, DROP…), found in a psql or sqlite3 query.
	Write string
}

// SQLRead reports whether a command is one read-only database query: psql
// or sqlite3 running a single SELECT-like statement, maybe piped into text
// tools. CloudBuddy then runs psql in a read-only session; sqlite3 must open
// the file -readonly.
func SQLRead(command string) SQLCheck {
	segs := expand(splitShell(command))
	if len(segs) == 0 || (segs[0].Prog != "psql" && segs[0].Prog != "sqlite3") {
		return SQLCheck{Why: "it isn't psql or sqlite3"}
	}
	first := segs[0]
	check := SQLCheck{}
	no := func(why string) SQLCheck { check.Why = why; return check }
	sqls := sqlArgs(first)
	for _, q := range sqls {
		if w := sqlWriteWord(q); w != "" {
			check.Write = w
			return no("the query has " + strings.ToUpper(w) + " in it")
		}
	}
	if !PlainShell(command) {
		return no("it uses shell features CloudBuddy can't read")
	}
	if writesFiles(command) {
		return no("it writes to a file")
	}
	if first.Opaque || first.Nested || first.Background {
		return no("CloudBuddy can't read how it runs")
	}
	if strings.Contains(first.ProgPath, "/") {
		return no("it names a copy of the program by its path")
	}
	for _, s := range segs[1:] {
		if !dashTextTools[s.Prog] || !textToolSafe(s) {
			return no("it pipes the result into " + progName(s) + " in a way CloudBuddy can't be sure only reads")
		}
	}
	switch first.Prog {
	case "psql":
		for k := range first.Env {
			if !pgEnvAllowed[k] {
				return no("it sets " + k)
			}
		}
		if why := psqlRead(first.Args); why != "" {
			return no(why)
		}
	case "sqlite3":
		if len(first.Env) > 0 {
			return no("it sets environment variables")
		}
		if why := sqliteRead(first.Args); why != "" {
			return no(why)
		}
	}
	check.Engine = first.Prog
	return check
}

// sqlArgs is every argument of a psql or sqlite3 command that could be a
// query: they are only checked for write words here.
func sqlArgs(s segment) []string {
	var out []string
	for _, a := range s.Args {
		if strings.ContainsAny(a, " \n") {
			out = append(out, a)
		}
	}
	return out
}

func sqlWriteWord(q string) string {
	for _, w := range sqlWords(q) {
		if sqlChanges[w] {
			return w
		}
	}
	return ""
}

// sqlChanges are the words of statements that change data or the schema,
// or run stored code: a query with one of them can never be a dashboard
// command. (SELECT … INTO makes a table; COPY can run a program.)
var sqlChanges = map[string]bool{"insert": true, "update": true, "delete": true, "merge": true, "upsert": true,
	"truncate": true, "drop": true, "alter": true, "create": true, "grant": true, "revoke": true, "into": true,
	"copy": true, "call": true, "do": true, "execute": true, "vacuum": true, "reindex": true, "cluster": true,
	"refresh": true, "attach": true, "detach": true}

// PlainShell reports whether a command uses only shell syntax CloudBuddy
// reads fully: words, quotes, plain $NAME variables, pipes, ; && ||, and
// redirections. Anything else can run code the words don't show (Codex
// review of D-067): zsh glob qualifiers like *(e:'rm x':), subshells and
// grouping, command, process and arithmetic substitution ($(…), `…`,
// <(…), =(…), $[…]), ${…} expansions, here-documents, and background &.
// Every dashboard read must pass it.
func PlainShell(cmd string) bool {
	rs := []rune(cmd)
	at := func(i int) rune {
		if i >= 0 && i < len(rs) {
			return rs[i]
		}
		return 0
	}
	dollarOK := func(i int) bool { // rs[i] == '$': only $NAME
		n := at(i + 1)
		return n == '_' || (n >= 'A' && n <= 'Z') || (n >= 'a' && n <= 'z')
	}
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch c {
		case '\\':
			i++
		case '\'':
			for i++; i < len(rs) && rs[i] != '\''; i++ {
			}
		case '"':
			for i++; i < len(rs) && rs[i] != '"'; i++ {
				switch rs[i] {
				case '\\':
					i++
				case '`':
					return false
				case '$':
					if !dollarOK(i) {
						return false
					}
				}
			}
		case '(', ')', '`', '{', '}':
			return false
		case '$':
			if !dollarOK(i) {
				return false
			}
		case '<':
			if at(i+1) == '<' || at(i+1) == '(' {
				return false
			}
		case '>':
			if at(i+1) == '(' {
				return false
			}
		case '&':
			// && and the stream copies 2>&1, >&2 are fine; a lone & runs in the background
			if at(i+1) == '&' {
				i++
			} else if at(i-1) != '>' && at(i-1) != '<' {
				return false
			}
		}
	}
	return true
}

// dashLocalTools only read files or print text: a command made of them alone
// is a read (when it reads nothing outside the project). textToolSafe checks
// the options that would make them write or show secrets.
var dashLocalTools = map[string]bool{"echo": true, "printf": true, "jq": true, "yq": true, "grep": true, "egrep": true,
	"fgrep": true, "head": true, "tail": true, "wc": true, "uniq": true, "cut": true, "tr": true, "column": true, "cat": true,
	"base64": true, "true": true, "ls": true, "pwd": true, "date": true, "stat": true, "du": true, "diff": true,
	"basename": true, "dirname": true, "realpath": true, "test": true, "[": true, "cd": true}

// textToolSafe reports whether one text tool, with these arguments, only
// reads and prints (Codex review of D-067: uniq in out, yq -i, base64 -o).
func textToolSafe(s segment) bool {
	if s.Opaque || s.Nested || s.Background || len(s.Env) > 0 || strings.Contains(s.ProgPath, "/") {
		return false
	}
	switch s.Prog {
	case "printf":
		return !hasFlag(s.Args, "-v") && !anyPrefix(s.Args, "-v")
	case "tail":
		for _, a := range s.Args {
			if a == "-f" || a == "-F" || strings.HasPrefix(a, "--follow") || (strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsAny(a, "fF")) {
				return false
			}
		}
	case "uniq":
		// uniq writes its second file name ("-" is a name too: standard input)
		names := 0
		for _, a := range s.Args {
			if a == "-" || !strings.HasPrefix(a, "-") {
				names++
			}
		}
		if names > 1 {
			return false
		}
	case "base64":
		for _, a := range s.Args {
			if a == "-o" || strings.HasPrefix(a, "--output") || (strings.HasPrefix(a, "-o") && len(a) > 2) {
				return false
			}
		}
	case "date":
		// date can set the clock: only formats and time zones
		for _, a := range s.Args {
			if !strings.HasPrefix(a, "+") && a != "-u" && a != "-R" && a != "--utc" && a != "--rfc-2822" && !strings.HasPrefix(a, "-I") && !strings.HasPrefix(a, "--iso-8601") {
				return false
			}
		}
	case "jq", "yq":
		for _, a := range s.Args {
			// in-place edits, and every environment variable (env, $ENV), secrets included
			if a == "-i" || strings.HasPrefix(a, "--inplace") || strings.HasPrefix(a, "--in-place") ||
				(strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "i") && s.Prog == "yq") ||
				strings.Contains(a, "env") || strings.Contains(a, "ENV") {
				return false
			}
		}
	}
	return true
}

// TextOnly reports whether a command only reads files and prints text, with
// tools like cat, grep, wc and jq.
func TextOnly(command string) bool {
	if !PlainShell(command) || writesFiles(command) {
		return false
	}
	segs := expand(splitShell(command))
	if len(segs) == 0 {
		return false
	}
	for _, s := range segs {
		if !dashLocalTools[s.Prog] || !textToolSafe(s) {
			return false
		}
	}
	return true
}

func progName(s segment) string {
	if s.Prog == "" {
		return "something CloudBuddy can't read"
	}
	return s.Prog
}

// psql options a read may use. The value says whether it takes a value.
var psqlShort = map[byte]bool{'t': false, 'A': false, 'X': false, 'q': false, 'w': false, 'x': false,
	'c': true, 'h': true, 'p': true, 'U': true, 'd': true, 'F': true, 'R': true, 'P': true}
var psqlLong = map[string]bool{"--tuples-only": false, "--no-align": false, "--no-psqlrc": false, "--quiet": false,
	"--no-password": false, "--expanded": false, "--csv": false, "--command": true, "--host": true, "--port": true,
	"--username": true, "--dbname": true, "--field-separator": true, "--record-separator": true, "--pset": true}

func psqlRead(args []string) string {
	var sqls, others []string
	positionals := 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "--"):
			name, val, hasVal := strings.Cut(a, "=")
			takes, ok := psqlLong[name]
			if !ok {
				return "psql option " + name + " isn't one CloudBuddy knows is safe"
			}
			if takes && !hasVal {
				if i+1 >= len(args) {
					return "psql option " + name + " has no value"
				}
				i++
				val = args[i]
			}
			if name == "--command" {
				sqls = append(sqls, val)
			} else if takes {
				others = append(others, val)
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			for j := 1; j < len(a); j++ {
				takes, ok := psqlShort[a[j]]
				if !ok {
					return "psql option -" + string(a[j]) + " isn't one CloudBuddy knows is safe"
				}
				if !takes {
					continue
				}
				val := a[j+1:]
				if val == "" {
					if i+1 >= len(args) {
						return "psql option -" + string(a[j]) + " has no value"
					}
					i++
					val = args[i]
				}
				if a[j] == 'c' {
					sqls = append(sqls, val)
				} else {
					others = append(others, val)
				}
				break
			}
		default:
			positionals++
			others = append(others, a)
		}
	}
	if positionals > 2 {
		return "psql has more arguments than a database and a user"
	}
	if len(sqls) != 1 {
		return "psql must run exactly one -c query"
	}
	for _, o := range others {
		lo := strings.ToLower(o)
		// a connection string or a service file can turn the read-only session back off, or name a pager program
		if strings.Contains(lo, "options") || strings.Contains(lo, "transaction") || strings.Contains(lo, "read_only") ||
			strings.Contains(lo, "pager") || strings.Contains(lo, "service") {
			return "a psql argument changes session settings"
		}
	}
	return readOnlySQL(sqls[0], "psql")
}

// sqlite3 options a read may use; the value says whether it takes a value.
var sqliteOpts = map[string]bool{"readonly": false, "json": false, "csv": false, "header": false, "noheader": false,
	"list": false, "line": false, "column": false, "box": false, "markdown": false, "table": false, "tabs": false,
	"quote": false, "batch": false, "bail": false, "safe": false, "separator": true, "newline": true, "nullvalue": true}

func sqliteRead(args []string) string {
	readonly, safe := false, false
	var pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			name := strings.TrimLeft(a, "-")
			takes, ok := sqliteOpts[name]
			if !ok {
				return "sqlite3 option " + a + " isn't one CloudBuddy knows is safe"
			}
			if name == "readonly" {
				readonly = true
			}
			if name == "safe" {
				safe = true
			}
			if takes {
				i++
			}
			continue
		}
		pos = append(pos, a)
	}
	if !readonly || !safe {
		return "sqlite3 must run with -readonly -safe (safe mode turns off writefile(), .shell and the other ways out)"
	}
	if len(pos) != 2 {
		return "sqlite3 needs the database file and one query"
	}
	file := strings.ToLower(pos[0])
	if strings.HasPrefix(file, "file:") || strings.Contains(file, "?") {
		return "the database is opened with a URI"
	}
	for _, line := range strings.Split(pos[1], "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), ".") {
			return "the query has sqlite3 dot-commands"
		}
	}
	return readOnlySQL(pos[1], "sqlite3")
}

var sqlFirst = map[string]bool{"select": true, "with": true, "values": true, "table": true, "show": true, "explain": true}

// sqlWrites are words that only appear in statements that change data or
// settings. A WITH query can hide an INSERT, UPDATE or DELETE.
var sqlWrites = map[string]bool{"insert": true, "update": true, "delete": true, "merge": true, "upsert": true,
	"truncate": true, "drop": true, "alter": true, "create": true, "grant": true, "revoke": true, "copy": true,
	"into": true, "attach": true, "detach": true, "pragma": true, "vacuum": true, "reindex": true, "analyze": true,
	"analyse": true, "lock": true, "call": true, "do": true, "execute": true, "prepare": true, "notify": true,
	"listen": true, "refresh": true, "cluster": true, "load": true, "import": true, "set": true, "reset": true,
	"begin": true, "commit": true, "rollback": true, "savepoint": true, "release": true}

// sqlRiskyCalls are functions that act outside the query even in a
// read-only session: other connections, server files, settings, or (in the
// sqlite3 shell) files on this computer.
var sqlRiskyCalls = []string{"pg_terminate_backend", "pg_cancel_backend", "pg_reload_conf", "pg_rotate_logfile",
	"pg_promote", "pg_switch_wal", "pg_create_", "pg_drop_", "pg_advisory", "pg_read_", "pg_ls_", "pg_file_",
	"pg_stat_reset", "pg_logical_", "pg_replication_", "pg_backup_", "pg_import_", "set_config", "dblink", "lo_",
	"query_to_", "xpath", "writefile", "readfile", "edit", "load_extension", "fts3_tokenizer", "sqlar_", "zipfile"}

// readOnlySQL checks that sql is one statement that only reads.
func readOnlySQL(sql, engine string) string {
	s := strings.TrimSpace(sql)
	s = strings.TrimSpace(strings.TrimSuffix(s, ";"))
	if s == "" {
		return "the query is empty"
	}
	for _, bad := range []string{";", "--", "/*", "$", "`"} {
		if strings.Contains(s, bad) {
			return "the query has " + bad + " in it, so CloudBuddy can't be sure it is one read"
		}
	}
	if engine == "psql" && strings.Contains(s, `\`) {
		return "the query has a psql backslash command"
	}
	words := sqlWords(s)
	if len(words) == 0 || !sqlFirst[words[0]] {
		return "the query doesn't start with SELECT or WITH"
	}
	for _, w := range words {
		if sqlWrites[w] {
			return "the query has " + strings.ToUpper(w) + " in it"
		}
		for _, risky := range sqlRiskyCalls {
			if w == risky || (strings.HasSuffix(risky, "_") && strings.HasPrefix(w, risky)) {
				return "the query calls " + w
			}
		}
	}
	return ""
}

// sqlWords lists the lower-case words of a query outside string literals
// and quoted names.
func sqlWords(s string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, strings.ToLower(cur.String()))
			cur.Reset()
		}
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\'':
			// a string literal: its words aren't SQL
			flush()
			for i++; i < len(rs); i++ {
				if rs[i] == '\'' {
					if i+1 < len(rs) && rs[i+1] == '\'' { // '' inside
						i++
						continue
					}
					break
				}
			}
		case c == '"':
			// a quoted name ("writefile", "pg_terminate_backend") is still a
			// name: keep its text as one word (Codex review of D-067)
			flush()
			for i++; i < len(rs); i++ {
				if rs[i] == '"' {
					if i+1 < len(rs) && rs[i+1] == '"' {
						cur.WriteRune('"')
						i++
						continue
					}
					break
				}
				cur.WriteRune(rs[i])
			}
			flush()
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'):
			cur.WriteRune(c)
		default:
			flush()
		}
	}
	flush()
	return out
}

// writesFiles finds output redirections other than to /dev/null or another
// stream, outside quotes.
func writesFiles(cmd string) bool {
	rs := []rune(cmd)
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '\'', '"':
			q := rs[i]
			for i++; i < len(rs) && rs[i] != q; i++ {
				if q == '"' && rs[i] == '\\' {
					i++
				}
			}
		case '\\':
			i++
		case '>':
			rest := string(rs[i+1:])
			rest = strings.TrimPrefix(rest, ">")
			rest = strings.TrimLeft(rest, " ")
			if strings.HasPrefix(rest, "&1") || strings.HasPrefix(rest, "&2") || strings.HasPrefix(rest, "/dev/null") {
				continue
			}
			return true
		}
	}
	return false
}

// DashboardWrites reports whether a command writes files: a redirection to
// a file, or tee, mkdir or touch.
func DashboardWrites(command string) bool {
	if writesFiles(command) {
		return true
	}
	for _, s := range expand(splitShell(command)) {
		switch s.Prog {
		case "tee", "mkdir", "touch":
			return true
		}
	}
	return false
}

// Programs lists the programs a command runs, as written ("psql",
// "./scripts/count.sh"), so the caller can check where each one lives.
func Programs(command string) []string {
	var out []string
	for _, s := range expand(splitShell(command)) {
		if s.ProgPath != "" {
			out = append(out, s.ProgPath)
		}
	}
	return out
}

// Argv lists each simple command's program and arguments as the shell will
// see them, quotes removed: what a dashboard approval hashes.
func Argv(command string) [][]string {
	var out [][]string
	for _, s := range expand(splitShell(command)) {
		if s.ProgPath == "" {
			continue
		}
		out = append(out, append([]string{s.ProgPath}, s.Args...))
	}
	return out
}
