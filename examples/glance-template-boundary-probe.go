// Original source-boundary experiment, 2026-09-30. Standard library only.
// It produces strings; no browser, application, network request or real data.
// Run: go run examples/glance-template-boundary-probe.go
package main

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"runtime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func render(source string, value any) (string, error) {
	t, err := template.New("probe").Parse(source)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, value); err != nil {
		return "", err
	}
	return out.String(), nil
}

func run() error {
	fmt.Printf("go=%s\n", runtime.Version())
	cases := []struct {
		name     string
		source   string
		value    any
		expected string
	}{
		{"text", `<p>{{.}}</p>`, `<b>demo</b>`, `<p>&lt;b&gt;demo&lt;/b&gt;</p>`},
		{"attribute", `<p title="{{.}}">label</p>`, `demo" onmouseover="void 0`, `<p title="demo&#34; onmouseover=&#34;void 0">label</p>`},
		{"url-string", `<a href="{{.}}">open</a>`, `javascript:demo`, `<a href="#ZgotmplZ">open</a>`},
		{"url-typed", `<a href="{{.}}">open</a>`, template.URL(`javascript:demo`), `<a href="javascript:demo">open</a>`},
		{"https-other-origin", `<a href="{{.}}">open</a>`, `https://outside.invalid/read`, `<a href="https://outside.invalid/read">open</a>`},
		{"html-string", `<section>{{.}}</section>`, `<img src="/synthetic-missing" onerror="void 0">`, `<section>&lt;img src=&#34;/synthetic-missing&#34; onerror=&#34;void 0&#34;&gt;</section>`},
		{"html-typed", `<section>{{.}}</section>`, template.HTML(`<img src="/synthetic-missing" onerror="void 0">`), `<section><img src="/synthetic-missing" onerror="void 0"></section>`},
		{"typed-url-attribute-escaping", `<a href="{{.}}">open</a>`, template.URL(`https://metrics.invalid/?x=a&y=b`), `<a href="https://metrics.invalid/?x=a&amp;y=b">open</a>`},
	}
	for _, c := range cases {
		out, err := render(c.source, c.value)
		if err != nil {
			return err
		}
		if out != c.expected {
			return fmt.Errorf("%s: got %q, want %q", c.name, out, c.expected)
		}
		fmt.Printf("%s: %s\n", c.name, out)
	}

	inner, err := render(`<p>{{.}}</p>`, `<b>demo</b>`)
	if err != nil {
		return err
	}
	outer, err := render(`<article>{{.}}</article>`, template.HTML(inner))
	if err != nil {
		return err
	}
	if outer != `<article><p>&lt;b&gt;demo&lt;/b&gt;</p></article>` {
		return fmt.Errorf("outer render changed the escaped fragment: %q", outer)
	}
	fmt.Printf("nested-typed-fragment: %s\n", outer)
	return nil
}
