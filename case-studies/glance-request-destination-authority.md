# Glance request destinations: configuration authority reaches beyond the dashboard

A dashboard that can query infrastructure APIs is also a network client with infrastructure reachability. I traced how Glance chooses those destinations, which inputs can change them, and why restricting an API hostname does not necessarily restrict the operations reached on that host. A local experiment demonstrates two different failures of destination scoping, then blocks them with a narrower request policy.

**Review and experiments: 30 September 2026.** Target: Glance **v0.8.6**, commit [`fbd9c1b6ae0c50ec015843f1bea48c400cddb518`](https://github.com/glanceapp/glance/tree/fbd9c1b6ae0c50ec015843f1bea48c400cddb518). This is a source-boundary review, distinct from my earlier dashboard TLS repair. The experiments use original standard-library Go code and synthetic loopback services.

## Start with the person who can change the request

There are three different actors worth distinguishing. Treating them all as “dashboard users” hides the important boundary.

| Actor | Controlled input | Relevant authority |
|---|---|---|
| Dashboard viewer | Browser requests to published pages | Reading rendered data; no demonstrated ability to replace a configured fetch URL |
| Configuration maintainer | Widget URLs, headers, methods, templates and included configuration | Choosing requests made with the dashboard process's network reachability and supplied credentials |
| Upstream API contributor | Fields in a response later used by a template | Potentially influencing a second request if the trusted template uses those fields as destinations |

Glance's custom API configuration explicitly supports URLs, headers, methods and subrequests. `CustomAPIRequest.initialize` turns these values into a server-side HTTP request. The destination is required to be reachable from the Glance server, which may have access unavailable to the viewer's browser. [Official configuration](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/docs/configuration.md#L1569-L1599), [request construction](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L139-L198).

A configured request to a private service is not, by itself, evidence of server-side request forgery. It is the intended operation of this feature. A security finding needs the additional link: a less trusted actor must be able to steer an authorized fetch beyond its intended destinations or operations. OWASP's SSRF prevention guidance makes this input-to-request boundary central to the threat model. [OWASP SSRF prevention guidance](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html).

## Configuration is executable network policy

The custom API template does more than format JSON. Its function map provides `newRequest`, `withHeader`, `withParameter`, `withBasicAuth` and `getResponse`. `getResponse` initializes and executes the request on the server. The official guide shows an API response identifier being used to construct a second request. [Helper implementation](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L646-L706), [documented consecutive request example](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/docs/custom-api.md#L363-L381).

That produces two separate edges to review:

```text
maintainer's configuration → primary API request
primary API response field → template-created secondary request
```

A literal base URL combined with a small, validated identifier can keep the second request within the original intention. A complete URL copied from `.JSON.String "next"` gives the response producer substantially more control. It can select another hostname, port, route or query. A token attached to that second request adds another consequence, but the reachability boundary exists even when no credential is attached.

The inspected `fetchCustomAPIResponse` path uses the ordinary configured HTTP client. It does not apply a widget-specific destination allowlist before sending the request. The clients also honor environment proxy settings, so the actual route can involve a proxy. This is a source observation about the reviewed path, not a claim that a remote viewer can submit arbitrary destinations. [Fetch path](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L239-L256), [client definitions](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-utils.go#L24-L40).

## Editing an included file can change privileged behavior

The authority is wider than a single YAML file. Glance supports relative or absolute `$include` paths and watches included files for configuration changes. Its parser reads those files and combines their contents before normal configuration interpretation. Environment values and mounted secret files can then populate configuration fields. [Include parser](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/config.go#L240-L302), [variable sources](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/config.go#L190-L230), [reload documentation](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/docs/configuration.md#L56-L70).

The operational implication is concrete: an account permitted to modify an included widget file can influence requests made by the process that loads it. Browser authentication does not reduce that file writer's authority. Store ownership, writable parent directories and deployment permissions are therefore part of a dashboard security review. Mounting a shared community-widget directory as writable by unrelated automation is a trust decision, even if the main configuration file is protected.

These are the application's documented configuration capabilities. I did not demonstrate a way for an ordinary dashboard viewer to obtain file-write privileges or escape a container.

## Reproduce two destination-policy gaps locally

The [original destination-policy probe](../examples/glance-destination-policy-probe.go) creates a metrics service and a service named `internal.invalid`. Both are loopback `httptest` listeners. A custom dial function maps only the two synthetic names to those listeners; unknown destinations fail locally. The transport has no environment proxy. There is no external DNS, infrastructure address or credential.

The first phase reads a synthetic index response containing a complete `next` URL and fetches that URL without constraining it. The second service receives the request. The next phase treats upstream data as a bounded identifier instead of a URL: `cpu-01` is accepted, while traversal, query syntax, an encoded slash and a complete URL are rejected before a request is constructed.

The final phases distinguish origins from operations. A read-only-looking route redirects to `/admin` on the same metrics hostname. A scheme-and-host comparison permits that redirect. A policy that also compares the original approved path and query rejects it before another `/admin` request is sent. Both custom policies preserve a ten-redirect ceiling.

Run from the repository directory:

```fish
run-bounded 300 go run examples/glance-destination-policy-probe.go
```

Actual output recorded on 30 September 2026, exit status 0:

```text
go=go1.27.1-X:nodwarf5
response-url: synthetic_internal_hits=1
bounded-id: "cpu-01" accepted=true
bounded-id: "../admin" accepted=false
bounded-id: "node?mode=write" accepted=false
bounded-id: "node%2Fadmin" accepted=false
bounded-id: "http://internal.invalid/admin" accepted=false
origin-only: same_host_admin_hits=1
route-policy: expected_error=Get "/admin": redirect rejected: approved route changed
route-policy: additional_admin_hits=0
```

`run-bounded` is my process-deadline wrapper; `go run examples/glance-destination-policy-probe.go` also works without it. Requests have five-second timeouts. The synthetic `/admin` handler only increments a counter and returns dummy JSON. Its name illustrates an operation boundary; no administrative action is implemented.

## Fix the authority expansion at its source

For a small dashboard, the most reviewable template is often a fixed mapping. Do not turn an API-supplied URL into a request when the desired operation is “read one of these known metrics.” An illustrative Glance template can select one literal route:

```gotemplate
{{ $id := .JSON.String "id" }}
{{ if eq $id "cpu-01" }}
  {{ $metric := newRequest "https://metrics.example.invalid/v1/status/cpu-01" | getResponse }}
  <p>{{ $metric.JSON.String "status" }}</p>
{{ else }}
  <p>Unknown metric</p>
{{ end }}
```

The fixed URL avoids interpreting a response value as URL syntax. For larger identifiers, an application-side adapter can accept a documented identifier grammar, construct the URL itself, and expose only the required read-only API shape. Escaping a string for a URL path solves syntax encoding; it does not decide whether the requested object or operation is authorized. [Go path-encoding contract](https://pkg.go.dev/net/url#PathEscape).

A maintainer-level change should apply the same request policy to primary requests, subrequests and template-created requests. Otherwise an alternate helper can bypass a policy added to only one entry point. Decide whether redirects are needed at all; a canonical metrics endpoint generally has little reason to redirect to another route. If redirects are allowed, review destination, method and credential scope on the actual redirected request. Go's client owns that follow-up behavior, rather than the original URL string alone. [Go HTTP client policy](https://pkg.go.dev/net/http#Client).

At the infrastructure boundary, restrict dashboard egress to the API services actually required and supply narrowly scoped read-only credentials. Review proxy-mediated routes separately. A hostname check does not enforce resolved-address policy, and a read-only API token does not stop unauthenticated requests to other reachable services. These controls address different parts of the authority chain.

## What this review establishes

The tagged source shows that trusted configuration and templates can initiate server-side requests. The original probe demonstrates complete response URLs reaching a synthetic second service and an origin-only check allowing a different route on the approved host. The route policy demonstrably stops that second request.

The experiment does not execute Glance, test its configuration parser, measure actual homelab reachability, model DNS rebinding, or establish an attacker-controlled field in an existing widget. Those would require evidence from the exact deployment. The practical review question is narrower and useful now: **which actor controls every part of a secondary request, and where is the intended destination and operation enforced?**
