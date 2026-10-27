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
| `reviewed` | Only on reads. Says a person checked that this really never changes anything. **Only reviewed reads run without asking.** |
| `runs_code` | The command builds or runs project code with network (deploy tools). It always asks, with a warning. |
| `product`, `title` | Text for the card. |
| `loss`, `loss_title` | Lines for the approval card ("What will be lost" / "What will happen"). |

## What asks and what doesn't

With the default rule ("Ask before anything I can't undo"):

- **Deletes, and anything tagged `irreversible`, `money` or `people`, always ask.** So does live mode.
- Creates and changes that an adapter recognizes run without asking and show on the canvas.
- Anything only the generic layer recognized asks, unless it is a reviewed read.

## Generated adapters

`adapters/aws-generated.yaml` is built from the service models that ship with the AWS CLI:

```
go run ./tools/adaptergen -models <aws-cli>/botocore/data -version <version> -out adapters/aws-generated.yaml
```

The generator labels an operation only when its first word is on a reviewed list (`Describe…`, `List…` and `Get…` are reads, `Delete…` and `Terminate…` are deletes, and so on). Anything else is left out, so it asks. Hand-written rules in `aws.yaml` win over generated ones.

## Testing an adapter

Add cases to `internal/policy/golden_test.go`: a real command in, the expected decision out. Then run `go test ./internal/policy/`.
