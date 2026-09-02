# Handover: top-level `ingress` block + `for` support

Status: design agreed, not implemented.
Driver: CEEMS tenant onboarding in `cms-k8s`.

## Why

`cms-k8s/cms/cms-public.kdef` carries one `ingress {}` block per registrable domain, all
pointing at the same `cms-public-svc`. They differ only in `name` and `hosts`; TLS, issuer,
class and the two haproxy annotations are already hoisted into `ingress_defaults` in
`root.kdef`. The list grows every time a CEEMS tenant brings a custom domain, and the plan is
to have cms-api emit that list from its `website_host` table and commit it, so ArgoCD picks it
up with no hand-editing.

That needs the ingress list to be **data**, driven by `--values`. It cannot be today.

## Why it cannot be today

Two independent blockers, both small:

1. **`for` bodies only accept whole workloads.** The inner schema in
   [`internal/parser/loop.go:60-68`](../internal/parser/loop.go#L60-L68) lists
   `deployment`, `daemonset`, `statefulset`, `cronjob`, `configmap`. Anything else fails
   `block.Body.Content(innerSchema)` with "Blocks of type ... are not expected here".
2. **`for` is only expanded at file top level.** `expandForBlocks` is called from
   `parseFileBody` ([`internal/parser/app.go:53`](../internal/parser/app.go#L53)) and nowhere
   else. Deployment bodies never see it.

The cms-public ingresses live *inside* `deployment "cms-public"`, so both blockers apply.

## Decision

Add a **top-level `ingress` block type** rather than teaching `for` to recurse into deployment
bodies.

Rationale: an Ingress is not part of a Deployment in Kubernetes, it only names a Service. The
nested form is a convenience. A top-level form is less invasive (no recursive `for` plumbing),
is independently useful for any repo where ingresses outnumber workloads, and composes with
`for` for free. The nested form stays exactly as it is; nothing existing changes behaviour.

### The free part

`extractBlocksSource` ([`loop.go:113-145`](../internal/parser/loop.go#L113-L145)) slices the
raw source of the entire `for` body off disk and hands it to `ParseBytes`, which routes through
`parseFileBody` and therefore `topLevelSchema`. `innerSchema` is only used as a gate: it
validates which block types may appear, and its parsed `innerContent.Blocks` is otherwise
unused.

So once `ingress` is a top-level block type, making it work inside `for` is **one line**:
add `{Type: "ingress", LabelNames: []string{"name"}}` to `innerSchema`. No re-parse plumbing.

## Implementation checklist

### 1. Type — `internal/types/app.go` / `internal/types/config.go`

`IngressConfig` ([`app.go:57-68`](../internal/types/app.go#L57-L68)) is reusable as-is except
it has no namespace of its own (it inherits the workload's today). Add:

```go
Namespace string // only set on top-level ingress blocks
```

Add to `KdefConfig` ([`config.go`](../internal/types/config.go)):

```go
Ingresses []IngressConfig
```

`ResourceName(workloadName, index)` and `CertificateSecretName()` need no change. For a
top-level block the label *is* the name, so `Name` is always populated and `ResourceName`
short-circuits on its first branch.

### 2. Parser — `internal/parser/app.go`

- Add `{Type: "ingress", LabelNames: []string{"name"}}` to `topLevelSchema`
  ([`app.go:89-102`](../internal/parser/app.go#L89-L102)).
- Add a `case "ingress":` to `parseBlocksFromBody` appending to `result.Ingresses`.
- Add `Ingresses []types.IngressConfig` to `FileResult` and merge it in the `forResults` loop
  at [`app.go:57-68`](../internal/parser/app.go#L57-L68).
- `parseIngressBlock` ([`app.go:274`](../internal/parser/app.go#L274)) already parses every
  attribute needed. Wrap it so the block label seeds `ingress.Name`, and add a `namespace`
  attribute to its schema. Keep an explicit `name` attribute winning over the label, or reject
  it as redundant — pick one and say so in the error.

### 3. Parser — `internal/parser/loop.go`

Add `{Type: "ingress", LabelNames: []string{"name"}}` to `innerSchema`
([`loop.go:60-68`](../internal/parser/loop.go#L60-L68)).

### 4. Parser — `internal/parser/parser.go`

- Merge `result.Ingresses` into `config.Ingresses` at both merge sites
  ([`parser.go:105`](../internal/parser/parser.go#L105) and
  [`parser.go:946`](../internal/parser/parser.go#L946)), and in the root-defs merge around
  [`parser.go:185`](../internal/parser/parser.go#L185) / `245`.
- `injectNamespace` ([`parser.go:279`](../internal/parser/parser.go#L279)): add a loop over
  `config.Ingresses` setting `Namespace` when empty. Without this, a top-level ingress in a
  `cms/` subproject renders with no namespace and lands in `default`.
- `applyIngressDefaults` ([`parser.go:960`](../internal/parser/parser.go#L960)): add
  `config.Ingresses` to the three existing loops. The `apply` closure needs no change.
- `validateIngresses` ([`parser.go:433`](../internal/parser/parser.go#L433)): call `check`
  once more for the standalone list. Note `check` is keyed on `(kind, workload, index)` for
  its error messages, so pass something sane like `check("ingress", ing.Name, ing.Namespace, ...)`
  per block, or generalise the signature. **This is the part worth getting right** — it is the
  guard that stops two tenants deriving the same `<first-host>-tls` Certificate secret name,
  which is exactly the failure a generated list could introduce silently.

### 5. Generator — `internal/generator/generator.go`

Add to `Generate`, mirroring the deployment branch at
[`deployment_v2.go:220-254`](../internal/generator/deployment_v2.go#L220-L254):

```go
for _, ing := range config.Ingresses {
    result["ingress-"+ing.Name] = GenerateStandaloneIngress(ing, config.IngressDefaults)
}
```

The new function builds the same `types.AppConfig` shim the deployment path builds, then calls
`GenerateHTTPRoute` or `GenerateIngress` depending on `ingressDefaults.Mode`, plus
`GenerateCertificate`. Roughly:

```go
appCompat := types.AppConfig{
    Name:      ing.Name,
    Namespace: ing.Namespace,
    Ingress:   &ing,
}
```

Two behaviours to decide, both currently derived from the workload:

- **`app.kubernetes.io/name` label.** `GenerateIngress`
  ([`ingress.go:78`](../internal/generator/ingress.go#L78)) and `GenerateCertificate`
  ([`certificate.go:44`](../internal/generator/certificate.go#L44)) both stamp `app.Name`.
  Recommend using `ServiceName` when set, falling back to the block name, so a standalone
  ingress still groups with the workload it fronts under label selectors.
- **Backend port default.** `GenerateIngress`
  ([`ingress.go:35-40`](../internal/generator/ingress.go#L35-L40)) falls back to the workload's
  first container port, then 80. A standalone block has no workload, so it silently gets 80.
  Recommend making `port` and `service_name` **required** on a top-level ingress and erroring
  in the parser if absent, rather than defaulting to 80 and producing a wrong backend.

### 6. Surfaces that enumerate block types

Grep turned up four files listing block types that will need the new one for parity:

- `internal/lsp/schema.go`, `internal/lsp/completion.go`
- `vscode-extension/syntaxes/kdef.tmLanguage.json`, `vscode-extension/snippets/kdef.json`

`internal/importer/` also maps live Ingresses back to `.kdef`
([`mapper.go:100-161`](../internal/importer/mapper.go#L100-L161)); it groups them under the
workload owning the matching Service. Leaving `import` alone is fine for now — worth a note in
the release notes that imports still produce nested blocks.

### 7. Docs

`docs/block-types.md` gets an `## ingress — standalone Ingress` section (the block-type list
there is the doc index people actually read), and `docs/conditionals-and-loops.md` gains an
ingress example, since that is the whole point of the change.

## Tests

`internal/parser/ingress_validate_test.go` is the natural home for the collision cases. Cover:

- Top-level ingress renders with `ingress_defaults` applied (TLS, issuer, class, merged annotations).
- Namespace injection from the enclosing subproject's `root.kdef` entry.
- Collision between a top-level ingress and a nested one, both by resource name and by derived
  Certificate secret name.
- `for` over a values list producing N ingresses, asserting each gets distinct names.
- `port` / `service_name` omitted produces a parse error, not a silent `:80` backend.

## Consumer side: what `cms-k8s` looks like after

`cms/cms-public.kdef` keeps the `ceems.nl` wildcard block nested and hand-written (it is the
only one on the DNS-01 issuer, and it is not tenant data). Every custom-domain block moves out
to a new file:

```hcl
# cms/cms-ingresses.kdef
for "site" "var.sites" {
  ingress "cms-${site.name}" {
    service_name = "cms-public-svc"
    port         = 80
    hosts        = site.hosts
  }
}
```

The two haproxy body-size annotations move from each block up into `ingress_defaults` in
`root.kdef`, since all of them set the identical pair.

`sites.json`, generated by cms-api and committed:

```json
{
  "sites": [
    { "name": "cor-it-nl", "hosts": ["cor-it.nl", "www.cor-it.nl", "cor-it.be", "www.cor-it.be", "cor-it.net", "www.cor-it.net", "corit.nl", "www.corit.nl"] },
    { "name": "gsid-nl", "hosts": ["gsid.nl", "www.gsid.nl"] }
  ]
}
```

Hosts are listed in full, including `www.`, rather than derived in kdef. Two reasons: there is
no `concat` in kdef's function map ([`variables.go:406-412`](../internal/parser/variables.go#L406-L412)
registers only `secret`, `configmap`, `field_ref`, `file`, `image`), and each `www.` host is
already a real `website_host` row in the CEEMS database, so deriving it would mean kdef and the
database disagreeing about what exists.

`hosts[0]` determines the Certificate secret name, so the generator must emit a **stable
first element** — order by the canonical host, not by row id.

No `variable "sites" {}` declaration is needed: `BuildEvalContext` seeds `varValues` from the
values file before walking declarations
([`variables.go:372-375`](../internal/parser/variables.go#L372-L375)), so undeclared keys land
in `var.` intact. Declaring it anyway gives a better error when `--values` is missing.

ArgoCD already supports this: the plugin command is
`kdef render --dir . ${KDEF_ENV:+--env $KDEF_ENV} ${KDEF_VALUES:+--values $KDEF_VALUES}`
(`cluster-config/argocd/kdef-cmp-config.yaml:18`), so the cms Application just needs
`KDEF_VALUES=sites.json` in its plugin env.

## Out of scope, but do not lose

- **DNS gating.** Auto-adding a host whose DNS does not point at the cluster fails the ACME
  Order and triggers a multi-hour cert-manager backoff. That is the exact failure the
  one-ingress-per-domain split was designed to contain, and generating the list from the
  database reintroduces it at scale. cms-api must run the equivalent of
  `cluster-config/scripts/dns-precheck.sh` and hold a host in a "pending DNS" state until it
  passes, rather than writing it straight into `sites.json`.
- **CORS.** Onboarding step 4 (adding the origin to `nelmio_cors.yaml` in cms-api) disappears
  if `allowed_origins` reads hostnames from the database instead of static YAML. Separate
  change, same motivation.
- **`concat` and friends.** Registering a few HCL stdlib collection functions would make
  values files terser across all projects. Not needed for this.
