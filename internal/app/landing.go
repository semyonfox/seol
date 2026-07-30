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
    .start-intro { max-width:38rem; color:var(--muted); }
    .routes { display:grid; grid-template-columns:1fr 1fr; margin-top:1.5rem; border:1px solid var(--line); border-radius:.45rem; background:var(--panel); overflow:hidden; }
    .route { min-width:0; padding:1.35rem; }
    .route + .route { border-left:1px solid var(--line); }
    .route-label { margin:0 0 .35rem; color:var(--accent); font-size:.72rem; font-weight:750; letter-spacing:.1em; text-transform:uppercase; }
    .route h3 { margin:0 0 .65rem; font-size:1.2rem; line-height:1.25; }
    .route pre { margin:1rem 0 0; background:var(--paper); }
    .route .note { margin-top:1rem; }
    .facts { display:grid; grid-template-columns:repeat(3, 1fr); gap:1.5rem; }
    .facts h3 { margin:0 0 .35rem; font-size:1rem; }
    .facts p { color:var(--muted); font-size:.94rem; }
    table { display:block; width:100%; overflow-x:auto; border-collapse:collapse; margin:1rem 0; font-size:.92rem; }
    th, td { padding:.55rem .65rem; border-bottom:1px solid var(--line); text-align:right; }
    th:first-child, td:first-child { text-align:left; }
    th { color:var(--muted); font-size:.78rem; letter-spacing:.04em; text-transform:uppercase; }
    .note { color:var(--muted); font-size:.94rem; }
    footer { padding:1.5rem 0 3rem; color:var(--muted); font-size:.9rem; }
    @media (max-width:38rem) { main { padding-top:3rem; } .facts, .routes { grid-template-columns:1fr; } .route + .route { border-left:0; border-top:1px solid var(--line); } section { padding:2.4rem 0; } }
  </style>
</head>
<body>
<main>
  <header>
    <div class="masthead"><p class="eyebrow">Temporary static hosting</p><img class="site-logo" src="/logo.svg" alt=""></div>
    <h1>Seol</h1>
    <p class="tagline">A pastebin for static sites. Publish a page, get a link, share it, and let it disappear.</p>
    <p class="intro">Turn a report, dashboard, diagram, demo, or set of docs into a temporary public link—without setting up a deployment.</p>
    <ul class="signals" aria-label="Key features">
      <li>Free &amp; open source</li>
      <li>No viewer account</li>
      <li>One-day default</li>
      <li>Static files only</li>
    </ul>
    <div class="hero-actions">
      <a class="button" href="#start">CLI quick start</a>
      <a href="#agent-start">Agent instructions</a>
    </div>
  </header>

  <section id="start" aria-labelledby="start-title">
    <h2 id="start-title">Publish your first page</h2>
    <p class="start-intro">Publishing happens from a terminal or coding agent—there is no web upload form. Ask this server's operator for a publisher token; if you <a href="#self-host">self-host</a>, you create it as <code>SEOL_TOKEN</code>. Viewing links needs no token or account.</p>
    <div class="routes">
      <div class="route" id="agent-start">
        <p class="route-label">For coding agents</p>
        <h3>Hand off the whole job</h3>
        <p>Give your agent the server, token, and this prompt. It can build the artifact and return the finished link.</p>
        <pre><code>Create a static site for the result.
Configure Seol once:

  npx @semyonfox/seol configure \
    --server {{.PublicBaseURL}} \
    --token TOKEN

Publish it with:

  npx @semyonfox/seol publish \
    --quiet DIRECTORY

Return the shareable URL only.
Do not publish private data.</code></pre>
      </div>
      <div class="route">
        <p class="route-label">From your terminal</p>
        <h3>Publish without installing</h3>
        <p>NPX downloads the matching Seol binary, verifies it, and caches it for later runs.</p>
        <pre><code>npx @semyonfox/seol configure \
  --server {{.PublicBaseURL}} \
  --token TOKEN

npx @semyonfox/seol publish ./report

<span class="result">Published: {{.PublicBaseURL}}/p/…/</span></code></pre>
        <p class="note">Prefer a native binary? Download one from <a href="https://github.com/semyonfox/seol/releases/latest">GitHub Releases</a>.</p>
      </div>
    </div>
    <p class="note">Publish a standalone HTML file, or a directory/ZIP with <code>index.html</code> at its root. Limits: 10 MiB compressed, 50 MiB extracted, 100 files.</p>
  </section>

  <section aria-labelledby="speed">
    <h2 id="speed">Fast from an agent or terminal</h2>
    <p>Measured on Linux x64 against the live services. Warm startup is the median of five runs; workflow timings include the network request.</p>
    <table>
      <thead><tr><th>Route</th><th>Cold start</th><th>Warm start</th><th>Publish</th><th>Update</th></tr></thead>
      <tbody>
        <tr><td>Native Seol</td><td>&lt;0.01s</td><td>&lt;0.01s</td><td>0.13s</td><td>0.11s</td></tr>
        <tr><td>BunX + Seol</td><td>0.93s</td><td>0.04s</td><td>0.16s</td><td>0.15s</td></tr>
        <tr><td>NPX + Seol</td><td>2.23s</td><td>0.68s</td><td>0.80s</td><td>0.78s</td></tr>
        <tr><td>NPX + PostPlan 0.0.4</td><td>3.64s</td><td>0.71s</td><td>1.20s</td><td>1.27s</td></tr>
      </tbody>
    </table>
    <p class="note">Cold runs used empty package and Seol binary caches, though operating-system and registry caches can still affect results. Treat these as one-machine measurements, not universal guarantees.</p>
  </section>

  <section aria-labelledby="how-it-works">
    <h2 id="how-it-works">How it works</h2>
    <div class="facts">
      <div><h3>Contained interactions</h3><p>Inline JavaScript can power buttons, filters, and charts inside the page. An opaque-origin sandbox blocks storage, networking, forms, framing, navigation, workers, and external scripts.</p></div>
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

  <section id="self-host" aria-labelledby="self-host-title">
    <h2 id="self-host-title">Run your own</h2>
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
