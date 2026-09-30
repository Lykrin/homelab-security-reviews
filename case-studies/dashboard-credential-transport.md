# Glance: tracing API credentials from configuration to browser output

**I removed eight unnecessary TLS-verification bypasses from a private Glance dashboard, then reviewed the fetch and rendering code to understand the remaining trust boundaries.** The result was verified HTTPS without breaking the widgets. The source review also explains why a dashboard shell returning 200, HTML escaping, and “server-side headers” each provide less assurance than they first appear to.

## Version and evidence

The source-review baseline is the official **Glance v0.8.6** tag, resolving to commit `fbd9c1b6ae0c50ec015843f1bea48c400cddb518`. All Glance source links below use that immutable revision. Source inspection explains the request and rendering mechanisms; the dated repair record separately supports the configuration outcome. [Reviewed upstream revision](https://github.com/glanceapp/glance/tree/fbd9c1b6ae0c50ec015843f1bea48c400cddb518).

| Evidence class | What I used | What it can establish |
|---|---|---|
| Recorded runtime observations | September 26 transport checks, rendered-response comparisons and repair completion record | What happened on the exercised deployment then |
| New static analysis | September 30 reading of tagged Glance source and official Go documentation | Which branches and data objects explain that behavior; where review must extend |
| New local probe | Original standard-library Go program, fake credentials and loopback test servers | Default cross-host redirect behavior and a same-origin rejection policy on the local Go runtime |
| Illustrative examples | Placeholder URLs, dummy headers and simplified templates | A way to repeat the reasoning without exposing private configuration |

The deployment results come from the recorded repair. The expanded review combines pinned source inspection with the original local redirect probe below.

## The confirmed configuration defect

Eight configured API fetches used `allow-insecure: true` while attaching `Authorization` or `x-api-key` credentials. Glance documents this option as ignoring invalid or self-signed certificates; its default is false. [Glance custom API configuration](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/docs/configuration.md#L1569-L1642).

The important distinction is encryption versus authentication of the peer. Go's `InsecureSkipVerify` disables certificate-chain and hostname verification. An interception-capable attacker could therefore terminate a connection presenting an untrusted certificate and receive the credential-bearing request. That is a threat consequence of the setting, not evidence that interception occurred in my lab. [Go TLS verification contract](https://pkg.go.dev/crypto/tls#Config).

The applied repair removed all eight bypass settings, preserving the existing HTTPS destinations and credential references. This is a sanitized representation, not a deployment file:

```diff
 - type: custom-api
   url: https://<approved-api-host>/<read-only-path>
-  allow-insecure: true
   headers:
     Authorization: <existing-credential-reference>
```

From the dashboard's own guest, normal certificate verification already succeeded for all five checked endpoint families. Unauthenticated requests returned the expected HTTP 401. This separated a working TLS path from the deliberately missing API authorization. A workstation-only check would have answered a different question because routing and trust stores can differ.

The authenticated candidate subsequently rendered its root and all four configured lazy-content responses with HTTP 200 and zero widget API error blocks. The completion record reports successful live widget rendering after the repair. Those are the historical acceptance results.

## Follow the credential, not just the YAML setting

The relevant request lifecycle is:

```text
configuration headers
  → CustomAPIRequest.initialize
  → http.Request.Header
  → fetchCustomAPIResponse chooses an HTTP client
  → HTTP response + parsed JSON become template data
  → custom template executes
  → rendered widget HTML reaches the page-content response
```

`CustomAPIRequest.initialize` builds the outgoing request, copies configured header values into it, and applies basic authentication when configured. This is an ordinary Go HTTP request created by the server, rather than browser JavaScript constructing an API call. [Request construction](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L177-L198).

`fetchCustomAPIResponse` chooses `defaultInsecureHTTPClient` when `AllowInsecure` is true, otherwise `defaultHTTPClient`. Both have a five-second timeout and use environment proxy settings. The insecure client explicitly sets `InsecureSkipVerify: true`, with no replacement verifier in that client definition. Removing the flag therefore changes the client selected for that fetch. [Selection branch](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L239-L256), [client definitions](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-utils.go#L24-L40).

This suggests a useful review order: inventory credential-bearing requests first, then check their destinations, transport settings and any redirects. A flag count alone misses the behavior of the request after the initial connection.

## Redirects are a second transport boundary

Neither client definition installs `CheckRedirect`, so Go's normal redirect policy applies. Go documents selective handling of credentials: headers such as `Authorization` are retained for the same domain and its subdomains, while omitted for unrelated domains. Arbitrary application headers are not automatically recognized as secrets. In particular, `x-api-key` is not among the specially handled header names in the official client implementation. [Go client redirect contract](https://pkg.go.dev/net/http#Client), [header-copy implementation](https://go.dev/src/net/http/client.go).

The source-derived implication is that a credential sent as `x-api-key` can accompany a followed redirect to a different host. Even for `Authorization`, a redirect to a subdomain or a different scheme on the same host deserves scrutiny. Restoring certificate verification answers who terminated an HTTPS connection; it does not express an allowlist of destinations for subsequent requests.

I did **not** observe a harmful redirect in the archived deployment. For an operator, the next useful question is whether each configured API path is canonical and whether any allowed redirect remains within the intended trust boundary. For a maintainer considering a dedicated credential-bearing client, a concrete hardening proposal is to reject redirects or allow only an explicitly approved HTTPS origin in `CheckRedirect`. This redirect policy is a proposed application hardening change.

### Reproduce the header distinction without an application or real credential

I wrote an [original redirect probe](../examples/glance-redirect-probe.go) using only Go's standard library. It creates two `httptest` servers bound to loopback, and maps the unrelated synthetic names `origin.invalid` and `destination.invalid` to those listeners with a custom `DialContext`. The transport has no proxy and never performs external DNS. Both supplied headers contain only `demo-only` values.

The first request uses Go's default redirect behavior. The destination must receive one request with `x-api-key`, without `Authorization`, or the program exits with an error. The second request installs a `CheckRedirect` comparison of scheme and host; the expected native client error must wrap the policy rejection, and the destination's hit counter must remain unchanged. This distinguishes removing a header from refusing a request entirely.

From the repository directory, run:

```fish
run-bounded 300 go run examples/glance-redirect-probe.go
```

Recorded output on **2026-09-30**, exit status 0:

```text
go=go1.27.1-X:nodwarf5
default: destination_hits=1 authorization_present=false x_api_key_present=true
same-origin: expected_error=Get "http://destination.invalid/final": redirect rejected: origin changed
same-origin: additional_destination_hits=0
```

`run-bounded` is the local process-deadline wrapper; readers without it can run `go run examples/glance-redirect-probe.go` directly. Each HTTP client request has a five-second timeout. The deliberately local HTTP setup isolates redirect/header semantics; it does not test TLS verification, Glance itself, or a deployed API. The recorded Go version is the probe runtime, not proof of the compiler used for the archived Glance image.

## “Server-side” is a location, not a secrecy guarantee

The response object passed to a custom template is richer than the API JSON: `customAPIResponseData` includes a full `*http.Response`. The renderer passes this response data to template execution. [Template data and execution](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L203-L212), [execution call](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L349-L361).

Go's response type exposes the client request through `Response.Request`. This makes its request headers reachable to a trusted template author; HTML escaping does not redact a credential printed as text. The official custom API guide already documents access to response fields and headers, so template authority must be part of the review. [Go response fields](https://pkg.go.dev/net/http#Response), [Glance response access examples](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/docs/custom-api.md#L289-L311).

For a synthetic request carrying `Authorization: demo-only`, these two expressions have very different confidentiality properties:

```gotemplate
<p>{{ .JSON.String "status" }}</p>
<p>{{ .Response.Request.Header.Get "Authorization" }}</p>
```

The second expression illustrates how the exposed request object can reveal an outgoing credential. This example was derived from source, not executed. A zero-match response comparison supports the checked templates; it cannot establish that all templates are incapable of printing secrets.

The practical boundary is control of the dashboard configuration. Treat community widgets as source code to review before giving them access to the dashboard's APIs and credentials.

## Escaping depends on how the template uses the data

Glance parses custom templates with Go's `html/template`. Its global function map also supplies `safeHTML`, `safeURL` and `safeCSS`, and these functions are added to the custom API function map. These helpers convert strings into types that assert trusted content. [Template construction](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L68-L77), [trusted-content helpers](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/templates.go#L15-L26), [function-map merge](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L710-L715).

Go's security model assumes trusted template authors and untrusted execution data. It warns that trusted HTML and URL types bypass normal filtering. The consequence is specific: ordinary text interpolation and a third-party string piped through `safeHTML` do not share the same protection. [Go template security model](https://pkg.go.dev/html/template#hdr-Security_Model), [trusted HTML type](https://pkg.go.dev/html/template#HTML).

Template review must also look beyond raw-HTML helpers. Glance provides `newRequest`, `withHeader`, `getResponse` and `withAllowInsecure` inside templates. A template can initiate another server-side fetch and opt out of TLS verification without an `allow-insecure` YAML field. Dynamic request destinations need the same review as top-level and subrequest URLs. These are documented configuration capabilities, not a demonstrated unauthorized request primitive. [Glance template request guide](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/docs/custom-api.md#L351-L381), [helper implementation](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L646-L706).

The archived review found ordinary text and attribute expressions, no raw-HTML helper use, and no dynamic URL sources in the inspected templates. That supports the exercised configuration, without generalizing to every Glance widget.

## The browser sees a lazy response, not only the shell

The frontend fetches `/api/pages/{slug}/content/`, reads it as text and inserts the rendered result into the page. The server registers a separate content handler, updates outdated widgets there and renders the page-content template. Checking only `/` therefore misses the data-bearing surface. [Frontend fetch](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/static/js/page.js#L6-L13), [HTML insertion](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/static/js/page.js#L749-L754), [content handler](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/glance.go#L334-L366).

The historical server-side comparison checked **ten known nonempty protected runtime values across five responses**: the root and all four configured lazy pages. It reported **zero matches**, without printing those values. Both dashboard aliases redirected unauthenticated root and lazy-content requests to the identity gate; the backend listener was source-restricted. These are bounded results. They do not prove absence of encoded or unknown secrets, establish a completed login flow, or show that other routes are equally protected.

There is another reason to inspect the widget itself: a dashboard HTTP 200 is not the upstream API's HTTP 200. Go's client does not treat an HTTP error status as a transport error. Glance's fetch path can return valid JSON from a non-2xx response to the template. A widget should inspect `.Response.StatusCode` before treating values as successful measurements. [Go request outcome contract](https://pkg.go.dev/net/http#Client.Do), [Glance JSON/status handling](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L258-L283).

```gotemplate
{{ if eq .Response.StatusCode 200 }}
  <p>{{ .JSON.String "status" }}</p>
{{ else }}
  <p>API unavailable: {{ .Response.StatusCode }}</p>
{{ end }}
```

Use the API's documented success statuses and schema for a real widget. This example teaches the distinction; it does not validate a particular API response.

## A review procedure another operator can use

1. Record the installed application version and image pin. Review that revision rather than assuming the default branch represents the deployed binary.
2. Inventory credential-bearing primary requests, subrequests and template-created requests. Include request destination construction, TLS bypass helpers and proxy configuration. Work on redacted copies; do not dump resolved environment values.
3. From the actual fetch environment, make a verified request without credentials to an approved read-only API path. Record TLS success separately from HTTP authorization failure.
4. Remove unnecessary bypasses in a candidate and exercise the actual widgets. Check their API statuses, expected measurements and error blocks, not only the shell status.
5. Review redirect destinations, template access to response/request metadata, and uses of trusted-content helpers. Keep templates controlled by trusted maintainers and credentials limited to the API permissions the widget needs.
6. Exercise every configured lazy-content page and compare known protected values locally, retaining counts only. Verify the edge gate on both the shell and content routes, and the backend's direct-access restrictions.
7. After an authorized deployment, repeat the affected acceptance check once and preserve the dated result. Separate later source analysis from the original repair date.

For step 3, replace the placeholder with your own approved URL. This Fish command sends no credential and does not follow redirects:

```fish
curl --silent --show-error --max-time 5 --output /dev/null --write-out 'http=%{http_code}\n' 'https://<approved-api-host>/<read-only-path>'
```

If certificate verification fails, inspect that native error and fix the trust path or certificate. An automatic insecure retry would conceal the very condition being reviewed.

## Result and remaining limits

The completed change restored certificate verification for eight credential-bearing fetch definitions while preserving successful dashboard rendering. The new source analysis identifies additional review points—redirect policy, template authority, trusted-content helpers and API-status interpretation—that a simple configuration diff would leave unexplained.

The local probe demonstrated forwarding of a synthetic API-key header by the standard-library client and rejection before the destination by an explicit origin policy. The other source-derived implications were not dynamically exercised.
