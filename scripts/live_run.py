#!/usr/bin/env python3
"""Drive a running CloudBuddy server through one chat (live agent tests).

usage: live_run.py <base-url> <launch-url> <agent> <workdir> <prompt> <answers-json>
answers-json maps a substring of a step's command to "approve" or "deny".
Prints every step at the end and exits non-zero on timeout.
"""
import http.cookiejar, json, sys, time, urllib.request

base, launch, agent, workdir, prompt, answers = sys.argv[1:7]
answers = json.loads(answers)
jar = http.cookiejar.CookieJar()
class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *a, **k): return None
op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar), NoRedirect)
try: op.open(launch)
except urllib.error.HTTPError as e:
    if e.code != 302: raise
csrf = None
def call(method, path, body=None):
    req = urllib.request.Request(base + path, method=method, data=json.dumps(body).encode() if body is not None else None)
    req.add_header("Content-Type", "application/json")
    if method != "GET":
        req.add_header("Origin", base); req.add_header("X-CB-CSRF", csrf)
    try:
        with op.open(req) as r:
            data = r.read()
            return r.status, (json.loads(data) if data else None)
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()
st, sess = call("GET", "/api/session"); csrf = sess["csrfToken"]
st, created = call("POST", "/api/conversations", {"agent": agent, "workdir": workdir, "text": prompt})
assert st == 200, (st, created)
cid = created["id"]; print("conversation", cid, flush=True)
decided = set(); deadline = time.time() + 600
while time.time() < deadline:
    st, d = call("GET", "/api/conversations/" + cid)
    for s in d["steps"]:
        a = s.get("approval")
        if a and a["state"] == "pending" and a["id"] not in decided:
            ans = next((v for k, v in answers.items() if k in s["command"]), "deny")
            print(f"  ASK  {s['command'][:90]!r}  reason={a['reason']!r} -> {ans}", flush=True)
            print("      ", call("POST", "/api/approvals/" + a["id"], {"decision": ans})[0], flush=True)
            decided.add(a["id"])
    if d["status"] in ("done", "failed", "interrupted"):
        break
    time.sleep(1)
print("status:", d["status"])
for s in d["steps"]:
    print(f"  {s['seq']:>2} {s['kind']:<5} {s['provider']:<8} {s['action']:<7} {s['status']:<15} {s['command'][:70]!r}")
    if s["kind"] == "cloud" and s["output"]:
        print("       out:", s["output"][:160].replace("\n", " | "))
for c in d["chat"]:
    if c["type"] in ("notice", "agent"):
        print(f"  [{c['type']}] {c.get('text','')[:300]}")
print("result:", d.get("result"))
sys.exit(0 if d["status"] in ("done",) else 1)
