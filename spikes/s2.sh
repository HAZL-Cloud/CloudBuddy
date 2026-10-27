#!/bin/bash
# Phase 0 spike S2: drive codex app-server with pinned approval settings.
# Usage: spikes/s2.sh <case> "<prompt>"   (env: ISOLATED_HOME=1, POLICY=..., MODEL=...)
set -u
ROOT=$(cd "$(dirname "$0")/.." && pwd)
CASE=$1; PROMPT=$2
RUN=$ROOT/spikes/run/s2-$CASE
rm -rf "$RUN"; mkdir -p "$RUN/work"
cd "$RUN"
export CB_GATE_TOKEN=spike-token
"$ROOT/bin/spike-gate" -log "$RUN/gate.jsonl" -portfile "$RUN/gate.port" 2> "$RUN/gate.err" &
GATE=$!
sleep 0.5
export CB_GATE_URL=http://127.0.0.1:$(cat "$RUN/gate.port")
if [ "${ISOLATED_HOME:-}" = 1 ]; then
  # A CloudBuddy-owned CODEX_HOME: the user's config, rules and MCP servers
  # are not loaded; only the login is shared.
  mkdir -p "$RUN/codex-home"; ln -s "$HOME/.codex/auth.json" "$RUN/codex-home/auth.json"
  export CODEX_HOME="$RUN/codex-home"
fi
MCP=$(cat <<EOF
{"cb": {"command": "$ROOT/bin/spike-mcpgw", "args": ["$ROOT/bin/spike-fakemcp"],
  "env": {"CB_GATE_URL": "$CB_GATE_URL", "CB_GATE_TOKEN": "spike-token", "MCPGW_LOG": "$RUN/mcpgw.log", "FAKEMCP_LOG": "$RUN/fakemcp.log"}}}
EOF
)
"$ROOT/bin/spike-codexdrv" -cwd "$RUN/work" -prompt "$PROMPT" -log "$RUN/codex.jsonl" -policy "${POLICY:-on-request}" -model "${MODEL:-}" -mcp "$MCP" > "$RUN/driver.out" 2>&1
kill $GATE 2>/dev/null
echo "== $CASE"
cat "$RUN/driver.out"
echo "-- files in work:"; ls -la "$RUN/work" | tail -n +4
echo "-- gate:"; python3 -I -c "
import json,sys
for l in open(sys.argv[1]):
    r=json.loads(l); q=r['request']
    print(' ', r['source'], r['decision'], json.dumps(q)[:220])
" "$RUN/gate.jsonl" 2>/dev/null
echo "-- fakemcp ran:"; cat "$RUN/fakemcp.log" 2>/dev/null
echo "-- final agent message:"; python3 -I -c "
import json,sys
last=''
for l in open(sys.argv[1]):
    m=json.loads(l)['msg']
    if m.get('method')=='item/completed' and m['params']['item'].get('type')=='agentMessage': last=m['params']['item'].get('text','')
print(' ', last[:600].replace(chr(10),' | '))
" "$RUN/codex.jsonl"
tail -3 "$RUN/codex.jsonl.stderr"
