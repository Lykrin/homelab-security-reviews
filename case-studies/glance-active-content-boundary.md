# Glance active content: where escaped API data becomes trusted HTML

I traced the rendering path of a Glance custom API widget through two server-side template passes and the browser's HTML insertion step. The central result is precise: normal interpolation protects its HTML context, but converting a value to trusted HTML or a trusted URL changes which protections apply. A second render does not repair an unsafe assertion made during the first one.

**Review and experiment: 30 September 2026.** Target: Glance **v0.8.6**, immutable revision [`fbd9c1b6ae0c50ec015843f1bea48c400cddb518`](https://github.com/glanceapp/glance/tree/fbd9c1b6ae0c50ec015843f1bea48c400cddb518). This review concerns browser content construction; it does not repeat the earlier TLS or request-destination experiments.

## Three different operations are often called escaping

A widget can display an API string as text, place it inside a quoted attribute, or use it as a navigation URL. These contexts require different transformations. Encoding quotation marks protects an attribute boundary. Filtering a URL scheme prevents an active scheme from becoming a link. Choosing which HTTPS origins the browser may contact is an additional policy decision.

Go's `html/template` selects escaping from the template's context. Its security model assumes trusted template authors and untrusted execution data. It is designed to preserve the structure the author wrote, not to remove arbitrary hostile markup that an author explicitly marks trusted. [Go template security model](https://pkg.go.dev/html/template#hdr-Security_Model).

That makes the trust transition more informative than a search for one dangerous word:

```text
API-controlled string
  → expression in a trusted template
  → ordinary string OR an asserted trusted content type
  → escaped or verbatim fragment
  → page-content HTML
  → browser DOM
```

The review question is: **where did the value acquire the authority to become markup or an active URL?**

## Trace both server-side render passes

Glance parses a custom API template with `html/template`, executes it against API response data, then converts the rendered buffer to `template.HTML`. This fragment is stored as `CompiledHTML`. The outer widget template includes that already rendered fragment, and the page-content template includes each widget's `Render` result. [Custom template compilation](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L68-L77), [execution and typed result](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L349-L361), [outer widget inclusion](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/templates/custom-api.html#L5-L7), [page composition](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/templates/page-content.html#L13-L20).

The typed result is reasonable when the first pass has already produced safe HTML: otherwise a widget's intended tags would become visible text. It is not an independent sanitization pass. If the first template emitted an event-handler attribute or an active URL through a trusted-content helper, the later typed inclusion carries it forward.

Conversely, conversion of a correctly escaped fragment to `template.HTML` does not decode an API string into an element. A string rendered as `&lt;b&gt;demo&lt;/b&gt;` remains text when included inside an outer HTML fragment. This distinction matters when evaluating reports that treat every `template.HTML` conversion as equivalent.

## The helpers change specific protections

Glance's global function map supplies `safeHTML`, `safeURL` and `safeCSS`. Each converts a string to the corresponding Go trusted-content type. The custom API function map imports these helpers. The names express an assertion by the template author; the implementation does not sanitize the input. [Helper definitions](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/templates.go#L15-L26), [custom API function merge](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-custom-api.go#L710-L715).

`safeHTML` permits the value to supply element and attribute syntax. `safeURL` bypasses URL scheme filtering, but does not disable every attribute-encoding step. A typed HTTPS URL containing `&` is still encoded appropriately in a quoted HTML attribute. These are separate transformations. Go documents the risks of both trusted types. [Go trusted HTML](https://pkg.go.dev/html/template#HTML), [Go trusted URL](https://pkg.go.dev/html/template#URL).

The same design assumption appears outside custom API widgets: the HTML widget stores its configured source as `template.HTML` and returns it directly. The widget header renders its configured title URL through `safeURL`. These are configuration-author facilities; reviewing only API values would miss the template author's own active-content authority. [HTML widget source](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/widget-html.go#L7-L19), [widget title URL](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/templates/widget-base.html#L1-L7).

## A reproducible contextual-escaping matrix

I wrote an [original template-boundary probe](../examples/glance-template-boundary-probe.go) using only the standard library. It renders synthetic strings through ordinary and typed contexts, checks the exact output, and repeats a safe fragment through an outer render. It does not run Glance or start a browser. No URL in the output is requested.

Run from the repository directory:

```fish
run-bounded 300 go run examples/glance-template-boundary-probe.go
```

Recorded output on 30 September 2026, exit status 0:

```text
go=go1.27.1-X:nodwarf5
text: <p>&lt;b&gt;demo&lt;/b&gt;</p>
attribute: <p title="demo&#34; onmouseover=&#34;void 0">label</p>
url-string: <a href="#ZgotmplZ">open</a>
url-typed: <a href="javascript:demo">open</a>
https-other-origin: <a href="https://outside.invalid/read">open</a>
html-string: <section>&lt;img src=&#34;/synthetic-missing&#34; onerror=&#34;void 0&#34;&gt;</section>
html-typed: <section><img src="/synthetic-missing" onerror="void 0"></section>
typed-url-attribute-escaping: <a href="https://metrics.invalid/?x=a&amp;y=b">open</a>
nested-typed-fragment: <article><p>&lt;b&gt;demo&lt;/b&gt;</p></article>
```

`run-bounded` is my process-deadline wrapper; the direct `go run` command works without it. The source is an output experiment, not an application vulnerability test.

| Result | What it establishes |
|---|---|
| Text output encodes angle brackets | API markup is displayed as text in that context |
| Attribute output encodes injected quotation marks | The apparent `onmouseover` text remains inside the original title attribute |
| Ordinary `javascript:` URL becomes `#ZgotmplZ` | The normal URL filter rejects that scheme |
| Typed URL preserves the active scheme | The trusted URL assertion changes scheme filtering |
| Ordinary foreign HTTPS link survives | Scheme filtering is not an origin allowlist |
| Typed HTML preserves an event attribute | The trusted HTML assertion permits markup structure |
| Typed URL still encodes `&` | Trusting a URL does not disable quoted-attribute encoding |
| Nested safe fragment retains its entities | Outer inclusion does not undo correct first-pass text escaping |

These observations make review recommendations testable. Saying only “the project uses Go templates” leaves out the decision that actually changes the boundary.

## The browser insertion step is not a sanitizer

Glance's frontend fetches the page-content response as text, then assigns it to `pageContentElement.innerHTML`. This turns the server's fragment into DOM structure. The HTML standard defines that operation through fragment parsing and replacement; it is not an application-specific content approval step. [Frontend content fetch](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/static/js/page.js#L6-L13), [DOM insertion](https://github.com/glanceapp/glance/blob/fbd9c1b6ae0c50ec015843f1bea48c400cddb518/internal/glance/static/js/page.js#L747-L768), [HTML standard: innerHTML](https://html.spec.whatwg.org/multipage/dynamic-markup-insertion.html#dom-element-innerhtml).

The probe establishes emitted bytes, not browser execution. In particular, do not infer that a `<script>` tag delivered through every HTML insertion mechanism behaves like a parser-loaded script. The stronger source-review question is whether untrusted data can provide any active markup, such as event-handler attributes or navigable active URLs, and what browser policy would constrain it. An authentication gate determines who can receive the content; it does not cleanse content after that user signs in.

## Patch the template where the assertion occurs

For an API field intended to be a description, removing the trusted-HTML assertion is the direct correction:

```diff
-<p>{{ .JSON.String "description" | safeHTML }}</p>
+<p>{{ .JSON.String "description" }}</p>
```

For an API-provided navigation URL, removing `safeURL` restores normal scheme filtering:

```diff
-<a href="{{ .JSON.String "url" | safeURL }}">Open metric</a>
+<a href="{{ .JSON.String "url" }}">Open metric</a>
```

The second change still allows ordinary HTTPS links to outside origins, as the probe demonstrates. If navigation must stay on one metrics service, construct the route from a fixed trusted base and a validated identifier, or map known identifiers to literal links. Do not describe the removal of `safeURL` as full destination authorization.

If rich HTML is a real requirement, place sanitization before the trusted-content conversion and define the allowed element, attribute and URL policies explicitly. Casting a string to `template.HTML` is the final assertion after that policy, not the policy itself. For a metrics dashboard with names, numbers and status strings, ordinary interpolation usually avoids that larger maintenance obligation.

## Review the content path rather than counting helpers

1. Identify who can control each API response field and each configuration file or included widget.
2. Mark every point where a value becomes trusted HTML, URL, CSS or attribute content.
3. Follow the resulting fragment through outer templates and the lazy-content response.
4. Exercise representative strings in their actual contexts, including quotes, markup and URL schemes. Inspect the produced structure; a dangerous-looking substring inside an encoded attribute is not the same as a new attribute.
5. Apply browser origin and content policies separately from encoding. Test the actual signed-in content path when evaluating a deployment.

The new experiment establishes contextual escaping and trusted-type behavior on the recorded local Go runtime. Source inspection establishes where the reviewed Glance revision exposes those capabilities. It does not show malicious input in a deployed widget, a browser exploit, or an unauthorized way to modify the configuration. The actionable boundary is the one the template author controls: **keep untrusted values as data until there is a specific, reviewable reason to grant them markup authority.**
