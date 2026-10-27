# Writing a CloudBuddy adapter

An adapter is one YAML file that teaches CloudBuddy about one provider: its CLI commands, its MCP tools and its HTTP API. Adapters make cards clearer and cut down prompts. They **never make anything less safe**: CloudBuddy's generic checks run on every call anyway, and the most cautious answer wins (plan decision D-026).

Without an adapter a service still works. CloudBuddy guesses from the command and asks before anything that isn't clearly read-only.

Built-in adapters live in `adapters/` and are compiled into the binary. You can also drop your own files into an `adapters/` folder next to the binary; a file with the same name as a built-in one is ignored.

## A small example

```yaml
provider: Stripe            # the name shown on cards and lanes
cli: [stripe]               # program names these rules apply to
live_flags: ["--live"]      # flags that mean "live / production"
products:                   # first subcommand -> product name on the card
  refunds: Refunds
rules:                      # first match wins
  - {match: ["*", list], action: read, reviewed: true}
  - match: [refunds, create]
    action: create
    tags: [money, irreversible]
    loss: ["Sends money back to the customer", "A refund can't be reversed"]
  - {match: [prices, create], action: create, live_tags: [irreversible]}
mcp:
  servers: [stripe]         # MCP server names containing this text
  tools:
    - {match: "list_*", action: read, reviewed: true}
    - {match: "*refund*", action: create, tags: [money, irreversible]}
http:
  hosts: [api.stripe.com]
  rules:
    - {method: GET, path: "*", action: read, reviewed: true}
    - {method: POST, path: "/v1/refunds*", action: create, tags: [money, irreversible]}
```

## Fields

**Rules (`rules`, `mcp.tools`, `http.rules`)**

| Field | Meaning |
|---|---|
| `match` | CLI: the words after the program, in order; `"*"` matches any one word, and extra words are fine. MCP: a glob on the tool name (`list_*`). |
| `method`, `path` | HTTP only. `path` is a glob; a trailing `*` matches any rest of the path. |
| `action` | `read`, `create`, `change`, `send`, `delete` or `unknown`. |
| `tags` | Any of `irreversible` (Can't undo), `money` (Moves money), `people` (Reaches people), `live`. |
| `live_tags` | Tags added only when the call is in live mode. |
| `reviewed` | On a read: a person checked it never changes anything. On a create or change: a person checked it is reversible and involves no money and no people. **Only reviewed rules run without asking**; anything else asks, even when the adapter names it. Not allowed on deletes, sends, `runs_code` rules, or rules with tags. |
| `runs_code` | The command builds or runs project code with network (deploy tools). It always asks, with a warning. |
| `product`, `title` | Text for the card. |
| `loss`, `loss_title` | Lines for the approval card ("What will be lost" / "What will happen"). |

## What asks and what doesn't

With the default rule ("Ask before anything I can't undo"):

- **Deletes, and anything tagged `irreversible`, `money` or `people`, always ask.** So does live mode.
- Reviewed reads, and creates and changes under a reviewed rule, run without asking and show on the canvas.
- Everything else asks: generated labels, unreviewed rules, and anything only the generic layer recognized.

## Generated adapters

`adapters/aws-generated.yaml` is built from the service models that ship with the AWS CLI:

```
go run ./tools/adaptergen -models <aws-cli>/botocore/data -version <version> -out adapters/aws-generated.yaml
```

The generator labels an operation only when its first word is on a reviewed list (`Describe…`, `List…` and `Get…` are reads, `Delete…` and `Terminate…` are deletes, and so on). It skips reads that mint credentials or reveal secrets (`GetSessionToken`, `GetSecretValue`…). It uses botocore's own naming rule, so command names match the AWS CLI.

Generated labels only describe cards: a generated read runs, but a generated create or change asks. To let a safe create run on its own, add a reviewed rule for it in `aws.yaml`. Hand-written rules win over generated ones.

## Testing an adapter

Add cases to `internal/policy/golden_test.go`: a real command in, the expected decision out. Then run `go test ./internal/policy/`.
