package dashmcp

// Tools are CloudBuddy's own MCP tools. The save_dashboard description is
// how the agent learns the widget kit (D-067), so it says exactly what
// CloudBuddy checks.
func Tools() []map[string]any {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	num := func(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
	enum := func(desc string, vals ...string) map[string]any {
		return map[string]any{"type": "string", "enum": vals, "description": desc}
	}
	source := map[string]any{
		"type":     "object",
		"required": []string{"key", "command"},
		"properties": map[string]any{
			"key":            str("Short id the tiles use, like \"users\" (lower-case letters, digits, - and _)."),
			"label":          str("What it fetches, in a few plain words."),
			"command":        str("A shell command, run like in the user's terminal in the project folder (login environment and direnv). It should print the data and nothing else: a number, JSON, CSV or text."),
			"cwd":            str("Folder to run in, relative to the chat's folder. Leave out for the chat's folder."),
			"format":         enum("How to read the output. auto (default) reads JSON, then a plain number, else text.", "auto", "json", "number", "csv", "tsv", "lines", "text"),
			"timeoutSeconds": num("Stop the command after this many seconds (default 60, at most 300)."),
		},
	}
	series := map[string]any{
		"type":     "object",
		"required": []string{"source"},
		"properties": map[string]any{
			"source": str("Source key."),
			"path":   str("Path to a number in the source's output."),
			"label":  str("Name of the line."),
		},
	}
	column := map[string]any{
		"type":     "object",
		"required": []string{"field"},
		"properties": map[string]any{
			"field":  str("Field name in each row."),
			"label":  str("Column heading."),
			"format": enum("How to show numbers.", "number", "currency", "percent", "bytes", "duration"),
		},
	}
	tile := map[string]any{
		"type":     "object",
		"required": []string{"type", "title"},
		"properties": map[string]any{
			"key":        str("Stable id for the tile (made from the title if left out). Keep it the same when you update a dashboard, so its history stays."),
			"type":       enum("number: one big number with its change and a sparkline. line: a number over time, or one point per row of the output. bars: label/value rows as bars. table: rows as a table. text: the output as text. status: OK / warning / failing.", "number", "line", "bars", "table", "text", "status"),
			"title":      str("Shown above the tile."),
			"source":     str("Source key (every tile but a line with series needs one)."),
			"path":       str("Where the value is in the source's output: data.length, data.0.count, items[2].name, rows.last.total. length/count give the size of a list. Leave out to use the whole output."),
			"width":      num("Columns out of 4 (number and status default to 1, table to 4, others to 2)."),
			"note":       str("A short line under the tile, like where the number comes from."),
			"format":     enum("How to show the number.", "number", "currency", "percent", "bytes", "duration"),
			"currency":   str("Currency code for format currency, like USD."),
			"decimals":   num("Decimal places."),
			"unit":       str("Word after the number, like \"users\"."),
			"good":       enum("Which way is good news, so a rise shows green or red.", "up", "down"),
			"compare":    enum("What a number's change is measured against (default 24h).", "previous", "1h", "24h", "7d"),
			"series":     map[string]any{"type": "array", "items": series, "description": "line: up to 6 numbers tracked over time, one line each."},
			"labelField": str("bars, line from rows: the field with the label (or x value)."),
			"valueField": str("bars, line from rows: the field with the number."),
			"columns":    map[string]any{"type": "array", "items": column, "description": "table: which fields to show, in order. Leave out to show all."},
			"limit":      num("bars, table: most rows to show."),
			"warnAbove":  num("status: warning above this number."),
			"failAbove":  num("status: failing above this number."),
			"warnBelow":  num("status: warning below this number."),
			"failBelow":  num("status: failing below this number."),
			"okValues":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "status: text values that mean OK, like [\"healthy\"]."},
		},
	}
	save := map[string]any{
		"name":  "save_dashboard",
		"title": "Save a dashboard",
		"description": `Save a dashboard the user can keep open in CloudBuddy. CloudBuddy reruns its commands itself on a timer, without you, and draws the tiles with its own widgets. Use it when the user wants numbers, charts or tables that stay fresh (users, signups, revenue, errors, deploy status). It returns what each command printed and what each tile shows: fix anything that failed and save again with the same id.

How to build one:
1. Find the data with shell commands first, and make each command print only the data: a number, JSON or CSV. Use jq, psql -tA or --csv, sqlite3 -readonly -safe -json, a CLI's --output json.
2. One source per command; tiles point at a source and, for JSON, a path into it.
3. A number tile records its value on every refresh, so its trend, sparkline and line chart fill in over time. For history that already exists (signups per day), return rows and use a line or bars tile with labelField and valueField.

What runs on the timer (CloudBuddy checks every command on every run):
- Runs by itself: cloud CLI reads (aws ... describe/list/get, stripe ... list/retrieve, gh ... list/view), text tools on project files (cat, grep, wc, jq), and database reads: psql with one SELECT in -c (CloudBuddy runs it in a read-only session, without ~/.psqlrc) and sqlite3 -readonly -safe with one SELECT. Only plain shell: words, quotes, $NAME variables, pipes, ; and &&; no $(...), ${...}, subshells or zsh glob qualifiers.
- Waits for the user's OK once, on the dashboard page: scripts and programs (node, python, npm run, ./bin/...), web requests (curl), anything that writes files or that CloudBuddy can't read. It asks again if the command or its script changes. Tell the user when this happens.
- Never runs from a dashboard: deletes, payments, messages to people, anything that can't be undone or that changes live mode, background commands. Inside a script, a CLI call like that is stopped.

Example: {"title": "Users", "refresh": "15m", "sources": [{"key": "users", "command": "psql \"$DATABASE_URL\" -tAc 'select count(*) from users'"}, {"key": "daily", "command": "psql \"$DATABASE_URL\" --csv -c 'select date(created_at) as day, count(*) as signups from users group by 1 order by 1 desc limit 30'", "format": "csv"}], "tiles": [{"type": "number", "title": "Users", "source": "users", "unit": "users", "good": "up"}, {"type": "line", "title": "Users over time", "source": "users"}, {"type": "bars", "title": "Signups per day", "source": "daily", "labelField": "day", "valueField": "signups"}]}`,
		"inputSchema": map[string]any{
			"type":     "object",
			"required": []string{"title", "refresh", "sources", "tiles"},
			"properties": map[string]any{
				"id":          str("Leave out to make a new dashboard; give an existing id (from list_dashboards) to replace that one."),
				"title":       str("Short name, like \"Users\"."),
				"description": str("One line on what it shows."),
				"refresh":     str("How often CloudBuddy reruns the commands: 1m, 5m, 15m, 30m, 1h, 6h, 12h or 1d."),
				"sources":     map[string]any{"type": "array", "items": source, "description": "The commands that fetch the data (at most 12)."},
				"tiles":       map[string]any{"type": "array", "items": tile, "description": "The tiles, in reading order, 4 columns wide (at most 24)."},
			},
		},
		"annotations": map[string]any{"title": "Save a dashboard", "readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false},
	}
	list := map[string]any{
		"name":        "list_dashboards",
		"title":       "List dashboards",
		"description": "List the dashboards in CloudBuddy: their ids, commands and tiles. Use it before changing one.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		"annotations": map[string]any{"title": "List dashboards", "readOnlyHint": true, "openWorldHint": false},
	}
	get := map[string]any{
		"name":        "get_dashboard",
		"title":       "Read a dashboard",
		"description": "Read one dashboard's full definition and what it shows now. Change the definition and send it to save_dashboard with the same id to update it.",
		"inputSchema": map[string]any{"type": "object", "required": []string{"id"}, "properties": map[string]any{"id": str("The dashboard's id.")}},
		"annotations": map[string]any{"title": "Read a dashboard", "readOnlyHint": true, "openWorldHint": false},
	}
	return []map[string]any{save, list, get}
}
