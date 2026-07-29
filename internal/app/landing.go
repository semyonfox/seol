package app

import (
	"bytes"
	"html/template"
	"net/http"
)

var landingPageTemplate = template.Must(template.New("landing").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="description" content="Seol is a temporary pastebin for static sites and agent-made artifacts. Publish a page and get a shareable link.">
  <link rel="icon" href="/favicon.svg" type="image/svg+xml">
  <title>Seol — temporary links for static sites</title>
  <style>
    :root { color-scheme: light; --paper:#faf7f2; --ink:#201d19; --muted:#6d655c; --line:#ddd5ca; --panel:#f1ece4; --accent:#b84c20; }
    * { box-sizing:border-box; }
    html { font-size:17px; }
    body { margin:0; background:var(--paper); color:var(--ink); font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; line-height:1.6; }
    main, footer { width:min(44rem, calc(100% - 2rem)); margin-inline:auto; }
    main { padding:5rem 0 3rem; }
    header { padding-bottom:3.5rem; border-bottom:1px solid var(--line); }
    .masthead { display:flex; align-items:flex-start; justify-content:space-between; gap:1rem; margin-bottom:.25rem; }
    .site-logo { width:clamp(3.5rem, 9vw, 4.5rem); height:auto; flex:none; }
    h1, h2 { letter-spacing:-.035em; line-height:1.1; }
    h1 { margin:.25rem 0 .75rem; font-size:clamp(3.4rem, 12vw, 6.6rem); }
    h2 { margin:0 0 1rem; font-size:1.5rem; }
    p { margin:.6rem 0; }
    .eyebrow { margin:0; color:var(--accent); font-size:.78rem; font-weight:750; letter-spacing:.12em; text-transform:uppercase; }
    .tagline { max-width:38rem; margin:0; font-size:clamp(1.45rem, 4vw, 2rem); line-height:1.35; }
    .intro { max-width:40rem; margin-top:1.25rem; color:var(--muted); font-size:1.05rem; }
    .signals { display:flex; flex-wrap:wrap; gap:.55rem; margin:1.5rem 0 0; padding:0; list-style:none; }
    .signals li { padding:.28rem .65rem; border:1px solid var(--line); border-radius:999px; color:var(--muted); font-size:.82rem; }
    .hero-actions { display:flex; flex-wrap:wrap; align-items:center; gap:1rem; margin-top:1.75rem; }
    .button { display:inline-block; padding:.6rem .9rem; border-radius:.35rem; background:var(--ink); color:var(--paper); font-weight:700; text-decoration:none; }
    .button:hover { text-decoration:none; background:var(--accent); }
    section { padding:3rem 0; border-bottom:1px solid var(--line); }
    a { color:var(--accent); text-decoration-thickness:.08em; text-underline-offset:.18em; }
    a:hover { text-decoration-thickness:.14em; }
    a:focus-visible { outline:3px solid var(--accent); outline-offset:4px; border-radius:2px; }
    pre { margin:1rem 0; padding:1.1rem 1.2rem; overflow-x:auto; border:1px solid var(--line); border-radius:.45rem; background:var(--panel); font:0.88rem/1.65 ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace; }
    code { font-family:ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace; }
    :not(pre) > code { padding:.1em .3em; border-radius:.2rem; background:var(--panel); font-size:.9em; }
    .result { color:var(--accent); }
    .facts { display:grid; grid-template-columns:repeat(3, 1fr); gap:1.5rem; }
    .facts h3 { margin:0 0 .35rem; font-size:1rem; }
    .facts p { color:var(--muted); font-size:.94rem; }
    .handoff { display:grid; grid-template-columns:minmax(0, .8fr) minmax(0, 1.2fr); gap:2rem; align-items:start; }
    .handoff pre { margin:0; }
    .note { color:var(--muted); font-size:.94rem; }
    footer { padding:1.5rem 0 3rem; color:var(--muted); font-size:.9rem; }
    @media (max-width:38rem) { main { padding-top:3rem; } .facts, .handoff { grid-template-columns:1fr; gap:.8rem; } section { padding:2.4rem 0; } }
  </style>
</head>
<body>
<main>
  <header>
    <div class="masthead"><p class="eyebrow">Temporary static hosting</p><img class="site-logo" src="/logo.svg" alt=""></div>
    <h1>Seol</h1>
    <p class="tagline">A pastebin for static sites. Publish a page, get a link, share it, and let it disappear.</p>
    <p class="intro">Seol is the quick handoff between a generated artifact and the person who needs to see it. Give the command to a coding agent—or run it yourself—to share reports, dashboards, diagrams, demos, and docs without setting up a deployment.</p>
    <ul class="signals" aria-label="Key features">
      <li>Free &amp; open source</li>
      <li>No account needed to view</li>
      <li>Expires automatically</li>
      <li>Static files only</li>
    </ul>
    <div class="hero-actions">
      <a class="button" href="#quick-start">Publish something</a>
      <a href="#agent-handoff">Give it to an agent</a>
    </div>
  </header>

  <section id="agent-handoff" aria-labelledby="agent-handoff-title">
    <div class="handoff">
      <div>
        <h2 id="agent-handoff-title">Give this to your agent</h2>
        <p>Let it make the artifact, publish it, and come back with a normal link you can open anywhere.</p>
      </div>
      <pre><code>Create a static site for the result.
Publish it with:

  seol publish --quiet DIRECTORY

Return the shareable URL.
Do not include private data.</code></pre>
    </div>
  </section>

  <section aria-labelledby="quick-start">
    <h2 id="quick-start">Install and publish</h2>
    <p>On Linux amd64:</p>
    <pre><code>mkdir -p ~/.local/bin
curl -fL https://github.com/semyonfox/seol/releases/latest/download/seol-linux-amd64 \
  -o ~/.local/bin/seol
chmod +x ~/.local/bin/seol</code></pre>
    <p class="note">Other platforms are on <a href="https://github.com/semyonfox/seol/releases/latest">GitHub Releases</a>. With Go installed, use <code>go install github.com/semyonfox/seol/cmd/seol@latest</code>.</p>
    <pre><code>seol configure --server {{.PublicBaseURL}} --token TOKEN
seol publish ./report
<span class="result">Published: {{.PublicBaseURL}}/p/…/</span></code></pre>
    <p>Standalone HTML files work directly. Directories and ZIP archives need an <code>index.html</code> at their root.</p>
    <p class="note">Uploads are limited to 10 MiB compressed, 50 MiB extracted, and 100 archive entries.</p>
  </section>

  <section aria-labelledby="how-it-works">
    <h2 id="how-it-works">How it works</h2>
    <div class="facts">
      <div><h3>Passive only</h3><p>HTML, CSS, images, fonts, JSON, and SVG are served as files. JavaScript and uploaded server-side code never run.</p></div>
      <div><h3>Random URLs</h3><p>Each page gets a cryptographically random public link. Publishing uses one configured server token; viewing needs none.</p></div>
      <div><h3>Temporary</h3><p>Pages live for one day by default and at most seven days after their latest update. Expired content is removed automatically.</p></div>
    </div>
  </section>

  <section aria-labelledby="commands">
    <h2 id="commands">The commands</h2>
    <pre><code>seol publish [--title TITLE] [--expires 7d] [--quiet|--json] PATH
seol list
seol stats
seol info PAGE_ID
seol replace PAGE_ID PATH
seol expiry PAGE_ID 3d
seol delete PAGE_ID</code></pre>
    <p class="note"><code>--quiet</code> prints only the URL for scripts and agents. <code>--json</code> returns machine-readable output.</p>
  </section>

  <section aria-labelledby="agent-setup">
    <h2 id="agent-setup">Set up Codex once</h2>
    <p>Ask Codex to use <code>$skill-installer</code> to install the <a href="https://github.com/semyonfox/seol/tree/main/skills/seol">Seol skill</a>. After that, you can simply ask it to publish any static artifact with Seol.</p>
  </section>

  <section aria-labelledby="self-host">
    <h2 id="self-host">Run your own</h2>
    <p>Seol is one Go binary with SQLite metadata and filesystem storage. The included Compose setup can run it with an optional Cloudflare Tunnel sidecar.</p>
    <pre><code>git clone https://github.com/semyonfox/seol.git
cd seol
cp .env.example .env
# Set SEOL_TOKEN in .env, then:
docker compose up --build -d</code></pre>
    <p class="note">See the <a href="https://github.com/semyonfox/seol#quick-start">README</a> for configuration and tunnel setup.</p>
  </section>
</main>
<footer><a href="https://github.com/semyonfox/seol">Source on GitHub</a> · MIT licensed · no accounts, dashboard, or uploaded server-side code.</footer>
</body>
</html>`))

func (s *Server) landingPage(w http.ResponseWriter, _ *http.Request) {
	var page bytes.Buffer
	if err := landingPageTemplate.Execute(&page, struct{ PublicBaseURL string }{s.cfg.PublicBaseURL}); err != nil {
		http.Error(w, "Could not render landing page.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = page.WriteTo(w)
}

func serveBrandAsset(data []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(data)
	}
}
