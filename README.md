# Sustain's provider for Terraform

Optymyse Sustain's monitors, services, alerting and status pages, kept in Terraform with the code they watch: a monitor
made with the service it watches, reviewed in the same pull request, the same in staging as in production. It manages
one workspace through its API ([Sustain's API](https://sustain.watch/api/v1/openapi.json)).

It needs Terraform 1.11 or later, as a secret's value, a channel's address and a page's password are write-only
arguments: sent to Sustain, never written to a plan or the state.

## Using it

```hcl
terraform {
  required_providers {
    sustain = { source = "optymyse/sustain" }
  }
}

# The workspace's address, and an API token from Settings › API tokens; SUSTAIN_URL and SUSTAIN_TOKEN when not given.
provider "sustain" {
  url = "https://acme.sustain.watch"
}

resource "sustain_http_monitor" "checkout" {
  name = "Checkout"
  config = {
    url             = "https://shop.example.com/health"
    expected_status = { mode = "range", min_status_code = 200, max_status_code = 299 }
    assertions = [
      {
        id        = "no-store"
        type      = "header"
        name      = "cache-control"
        condition = "equals"
        expected  = { source = "literal", value = "no-store" }
      },
    ]
  }
  places_to_fail = 3
}
```

A token of scope `write` manages monitors, services, secrets and maintenance; alerting, clients and status pages are an
Admin's, so they need `admin`. A token acts as the person who made it, never beyond its scope.

## Resources

Every type of monitor, one resource each: `sustain_http_monitor`, `sustain_tcp_monitor`, `sustain_udp_monitor`,
`sustain_dns_monitor`, `sustain_ping_monitor`, `sustain_mail_monitor`, `sustain_grpc_monitor`,
`sustain_browser_monitor` and `sustain_heartbeat_monitor`. Then `sustain_service`, `sustain_secret`, `sustain_client`,
`sustain_alert_policy`, `sustain_alert_channel`, `sustain_alert_contact`, `sustain_on_call_schedule`,
`sustain_status_page`, `sustain_repeating_maintenance` and `sustain_private_location`. Each is imported by its id:
`terraform import sustain_service.shop svc_…`.

Two data sources name what was made elsewhere, by its id or its name, so a configuration can use it:
`sustain_monitor` and `sustain_service`.

What happens rather than how the workspace is set up isn't a resource: incidents, deploys, one-off maintenance,
subscribers, people and tokens.

- **A change made elsewhere** shows in the next plan, as Sustain answers it. A monitor's change names the version it was
  last read at, so one made in the console since is refused, never overwritten.
- **Sustain's own way of writing a value**, as a TCP monitor's hex in lower case, is kept as written in the plan, so it
  never shows as a change.
- **What Sustain gives when a field is left out** is planned as Sustain's, where Sustain's description says it: a
  monitor's SLA target, 99.9, and its domain's and certificate's alerts, 30 and 14 days before they expire. Those may be
  none in Sustain, which Terraform can't ask for, as it reads null as not given.
- **A shape chosen by its tag**, as an assertion by its `type`, is one object with every shape's fields, and is held to
  its tag's shape when it's planned, as Sustain's API would hold it.
- **A service's `depends_on`** is `dependency_ids` here, as Terraform keeps the name for its own.

## How it's made

Each resource's schema is generated from Sustain's own description of its API, so it can't drift from it, and the
provider makes, reads, changes and deletes every resource alike, as that description and
[`tools/resources.mjs`](tools/resources.mjs) say it. `make generate`, whenever Sustain's API changes:

1. **Sustain's description**, OpenAPI 3.1, kept as [`api/sustain.json`](api/sustain.json): a workspace's
   `/api/v1/openapi.json`.
2. **The provider's copy**, [`api/provider.json`](api/provider.json), made by [`tools/derive.mjs`](tools/derive.mjs)'s
   five general rules, so a new endpoint or field follows with no change to them: a union of literals as a list of
   values; a value or null as the value, marked as one that may be null; shapes chosen by a tag as one shape, each kept
   in [`api/tags.json`](api/tags.json); an answer wrapping its object as the object; and each resource's paths of its
   own, what it's made and changed with as one shape.
3. **HashiCorp's specification of it**, [`api/provider-spec.json`](api/provider-spec.json), by their OpenAPI generator,
   `tfplugingen-openapi` v0.3.0, reading [`api/generator.yml`](api/generator.yml).
4. **The schemas in Go**, [`internal/provider/resources_gen.go`](internal/provider/resources_gen.go), by
   [`tools/schemas.mjs`](tools/schemas.mjs), with what the provider needs to know of each beside: what's sent, what's
   sent as null, what a tag chooses. HashiCorp's framework generator would write schemas too, but names two nested
   shapes alike in one resource, which Go refuses.

What's generated is checked in, so the provider builds without running the generators, which are HashiCorp's
previews. The provider itself runs on HashiCorp's Plugin Framework, protocol 6.

## Testing

`make check` runs everything that needs no workspace: the tools' tests, the generators and the docs again with
nothing changed, and Go's format, vet and tests.

`make acceptance` makes, changes, imports and deletes resources with Terraform itself, in a writable local preview of
Sustain's on Enterprise's plan, never a real workspace: in Sustain's repository, `pnpm ui:preview --port 4188 --plan
enterprise --writable`; then here, `make acceptance`. Each step is planned again once it's applied, which must change
nothing. An alert contact is the one resource it leaves out, as making one emails it, which a preview never sends.
Every example in the docs is made as it's written, too. Terraform is the one `TF_ACC_TERRAFORM_PATH` names, or its
latest, downloaded for the run.

## Docs

[`docs/`](docs/), as the Terraform Registry shows them, are written by `make docs`, HashiCorp's `tfplugindocs`, from
each schema's descriptions, which are Sustain's own, and [`examples/`](examples/).

## Releasing

By hand from the owner's machine, as every Optymyse release is: a tag, `v0.1.0` first, then
`GPG_FINGERPRINT=… goreleaser release --clean`, which builds a zip for each system and processor the Registry asks
for, their SHA-256 sums, the Registry's manifest ([`terraform-registry-manifest.json`](terraform-registry-manifest.json),
protocol 6.0), and signs the sums with Optymyse's GPG key, an RSA one, as the Registry takes no other; all of it a
GitHub release ([`.goreleaser.yml`](.goreleaser.yml)). The Registry takes each release from there, and none is changed
once it's out. Its version says what it holds: `v0.x` while Sustain's API isn't versioned by its own policy, `v1.0.0`
once it is.

## Licence

The Mozilla Public License 2.0 ([`LICENSE`](LICENSE)), as HashiCorp's own providers and most others are: anyone may use
it, and whoever changes its files and shares the result shares those changes too.
