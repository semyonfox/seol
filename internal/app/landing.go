package app

import (
	"bytes"
	"html/template"
	"net/http"

	"github.com/semyonfox/seol/assets"
)

var landingPageTemplate = template.Must(template.New("landing").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="description" content="Seol gives static reports, demos and docs a temporary, shareable link. Publish from your terminal.">
  <link rel="icon" href="/favicon.svg" type="image/svg+xml">
  <title>Seol | temporary links for static sites</title>
  <style>
    :root { color-scheme:light; --paper:#faf7f2; --ink:#201d19; --muted:#6d655c; --line:#ddd5ca; --panel:#f1ece4; --accent:#b84c20; }
    * { box-sizing:border-box; }
    html { font-size:106.25%; }
    body { margin:0; background:var(--paper); color:var(--ink); font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; line-height:1.6; }
    main, footer { width:min(44rem,calc(100% - 2rem)); margin-inline:auto; }
    main { padding:4rem 0 2rem; }
    header { padding-bottom:2.5rem; border-bottom:1px solid var(--line); }
    h1, h2, h3 { letter-spacing:-.025em; text-wrap:balance; overflow-wrap:anywhere; }
    h1 { margin:.25rem 0 .75rem; font-size:clamp(2.8rem,12vw,6.6rem); line-height:1.1; }
    h2 { margin:0 0 1rem; font-size:1.5rem; line-height:1.3; }
    h3 { margin:1.5rem 0 .6rem; font-size:1.1rem; line-height:1.4; }
    p { margin:.6rem 0; text-wrap:pretty; }
    .masthead { display:flex; flex-wrap:wrap; align-items:center; justify-content:space-between; gap:1rem; }
    .site-logo { width:clamp(3.5rem,9vw,4.5rem); height:auto; flex:none; }
    .eyebrow { margin:0; max-width:100%; color:var(--accent); font-size:.78rem; font-weight:750; letter-spacing:.12em; text-transform:uppercase; overflow-wrap:anywhere; }
    .tagline { max-width:38rem; margin:0; font-size:clamp(1.45rem,4vw,2rem); line-height:1.4; }
    .intro { max-width:40rem; margin-top:1rem; color:var(--muted); font-size:1.05rem; }
    .signals { display:flex; flex-wrap:wrap; gap:.55rem; margin:1.25rem 0 0; padding:0; list-style:none; }
    .signals li { padding:.28rem .65rem; border:1px solid var(--line); border-radius:999px; color:var(--muted); font-size:.82rem; }
    .hero-actions { display:flex; flex-wrap:wrap; align-items:center; gap:.5rem; margin-top:1.5rem; }
    .button, .nav-link { display:inline-flex; align-items:center; min-height:44px; padding:.6rem .9rem; border-radius:.35rem; font-weight:700; }
    .button { background:var(--ink); color:var(--paper); text-decoration:none; }
    .button:hover { background:var(--accent); }
    section { padding:2.5rem 0; border-bottom:1px solid var(--line); }
    a { color:var(--accent); text-decoration-thickness:.08em; text-underline-offset:.18em; overflow-wrap:anywhere; }
    a:hover { text-decoration-thickness:.14em; }
    :focus-visible, pre:focus, [tabindex="-1"]:focus { outline:3px solid var(--accent); outline-offset:4px; border-radius:2px; }
    .skip-link { position:absolute; inset-block-start:.5rem; inset-inline-start:1rem; padding:.6rem .9rem; background:var(--paper); transform:translateY(-200%); }
    .skip-link:focus { transform:none; }
    pre { margin:1rem 0; padding:1rem; min-width:0; white-space:pre-wrap; overflow-wrap:anywhere; border:1px solid var(--line); border-radius:.45rem; background:var(--panel); font:.88rem/1.65 ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace; }
    code { font-family:ui-monospace,SFMono-Regular,Consolas,"Liberation Mono",monospace; }
    :not(pre) > code { padding:.1em .3em; border-radius:.2rem; background:var(--panel); font-size:.9em; overflow-wrap:anywhere; }
    .snippet { margin:1rem 0; }
    .snippet pre { margin:.4rem 0 0; }
    .snippet-label { display:flex; flex-wrap:wrap; align-items:center; justify-content:space-between; gap:.5rem; color:var(--muted); font-size:.9rem; }
    button { min-height:44px; padding:.45rem .7rem; border:1px solid var(--line); border-radius:.3rem; background:var(--paper); color:var(--ink); font:inherit; cursor:pointer; }
    button:hover { border-color:var(--accent); }
    [hidden] { display:none !important; }
    .result { border-inline-start:3px solid var(--accent); }
    .facts { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:1.5rem; }
    .facts h3 { margin:0 0 .35rem; font-size:1rem; }
    .facts p, .note { color:var(--muted); font-size:.94rem; }
    .handoff { display:grid; grid-template-columns:minmax(0,.8fr) minmax(0,1.2fr); gap:2rem; align-items:start; }
    .handoff .snippet { margin:0; }
    .status { color:var(--muted); font-size:.94rem; }
    .status:empty { margin:0; }
    footer { padding:1rem 0 3rem; color:var(--muted); font-size:.9rem; }
    footer > a, summary { display:inline-flex; align-items:center; min-height:44px; }
    summary { cursor:pointer; text-decoration:underline; text-underline-offset:.18em; }
    .privacy-toggle { display:flex; align-items:center; gap:.6rem; min-height:44px; padding:.5rem 0; }
    input[type="checkbox"] { width:1.2rem; height:1.2rem; flex:none; accent-color:var(--accent); }
    @media (max-width:38rem) { main { padding-top:2rem; } .facts, .handoff { grid-template-columns:1fr; gap:1rem; } section { padding:2rem 0; } }
  </style>
  <script src="/landing.js" defer></script>
</head>
<body>
<a class="skip-link" href="#main-content">Skip to content</a>
<main id="main-content" tabindex="-1">
  <header>
    <div class="masthead"><p class="eyebrow">Temporary static hosting</p><img class="site-logo" src="/logo.svg" alt=""></div>
    <h1>Seol</h1>
    <p class="tagline">Publish a page. Get a link. Share it, and let it expire.</p>
    <p class="intro">Share static reports, dashboards, demos and docs from your terminal. Publishing needs a server token. Anyone with the link can view the page.</p>
    <ul class="signals" aria-label="Key features">
      <li>Free &amp; open source</li>
      <li>No account needed to view</li>
      <li>Temporary by default</li>
      <li>Static files only</li>
    </ul>
    <nav class="hero-actions" aria-label="On this page">
      <a class="button" href="#quick-start">Install and publish</a>
      <a class="nav-link" href="#commands">Commands</a>
      <a class="nav-link" href="#agent-handoff-title">Agent handoff</a>
    </nav>
  </header>

  <section aria-labelledby="quick-start">
    <h2 id="quick-start" tabindex="-1">Install and publish</h2>
    <p>You need a publisher token from the person running this server. This page cannot issue one. You can also <a href="#self-host">run your own server</a>. Viewing a shared page needs no token.</p>
    <h3>1. Install the command</h3>
    <div class="snippet">
      <div class="snippet-label"><span id="install-label">Linux x64</span><button type="button" data-copy="install-command" aria-label="Copy Linux install commands" hidden>Copy</button></div>
      <pre tabindex="0" role="region" aria-labelledby="install-label"><code id="install-command">mkdir -p ~/.local/bin
curl -fL https://github.com/semyonfox/seol/releases/latest/download/seol_linux_x64 \
  -o ~/.local/bin/seol
chmod +x ~/.local/bin/seol
export PATH="$HOME/.local/bin:$PATH"</code></pre>
    </div>
    <p class="note">The PATH line applies to this terminal. In another terminal, add <code>~/.local/bin</code> to your PATH or run <code>~/.local/bin/seol</code> directly.</p>
    <p class="note">Other platforms are on <a href="https://github.com/semyonfox/seol/releases/latest">GitHub Releases</a>. With Go installed, use <code>go install github.com/semyonfox/seol/cmd/seol@latest</code>.</p>
    <h3>2. Configure once</h3>
    <p>Replace <code>TOKEN</code> with your publisher token locally. The command saves it on your device. Keep it out of shared pages.</p>
    <div class="snippet">
      <div class="snippet-label"><span id="configure-label">Connect to this server</span><button type="button" data-copy="configure-command" aria-label="Copy configuration command" hidden>Copy</button></div>
      <pre tabindex="0" role="region" aria-labelledby="configure-label"><code id="configure-command">seol configure --server {{.PublicBaseURL}} --token TOKEN</code></pre>
    </div>
    <h3>3. Publish your page</h3>
    <p>Replace <code>./report</code> with your HTML file, directory or ZIP archive. Directories and ZIPs need an <code>index.html</code> at their root.</p>
    <div class="snippet">
      <div class="snippet-label"><span id="publish-label">Publish from your terminal</span><button type="button" data-copy="publish-command" aria-label="Copy publish command" hidden>Copy</button></div>
      <pre tabindex="0" role="region" aria-labelledby="publish-label"><code id="publish-command">seol publish ./report</code></pre>
    </div>
    <p id="output-label">Example output. Share the printed URL:</p>
    <pre class="result" aria-labelledby="output-label"><code>Published: {{.PublicBaseURL}}/p/…/</code></pre>
    <p class="note">Anyone with this link can view and forward it. Do not upload credentials or private data.</p>
    <p class="note">Default upload limits are 10 MiB compressed, 50 MiB extracted and 100 archive entries. Self-hosted servers may set different limits.</p>
  </section>

  <section aria-labelledby="how-it-works">
    <h2 id="how-it-works">Prepare a page that works</h2>
    <div class="facts">
      <div><h3>Contained scripts</h3><p>Uploaded server-side code never runs. Contained inline JavaScript can power page-local interactions. External and module scripts, network requests, storage, forms and programmatic clipboard access are blocked.</p></div>
      <div><h3>Relative assets</h3><p>Use paths such as <code>assets/chart.svg</code>. A path starting with <code>/</code> points to the server root, outside your published page.</p></div>
      <div><h3>Temporary</h3><p>Pages expire after one day by default, with a seven-day maximum. A successful update refreshes the selected lifetime. Self-hosted servers may use different expiry limits.</p></div>
    </div>
    <p class="note">If publishing returns <code>ACTIVE_HTML</code>, remove the unsupported markup named in the error and try again. Keep scripts and their data inline. See <a href="https://github.com/semyonfox/seol#use">supported files and interactions</a> for the full rules.</p>
  </section>

  <section aria-labelledby="commands">
    <h2 id="commands" tabindex="-1">The commands</h2>
    <p>Publishing the same source path again updates its remembered active link on this configured client. Use <code>--new</code> for a separate link.</p>
    <pre tabindex="0" role="region" aria-label="Publishing and management commands"><code>seol publish ./report
seol publish --new ./report
seol publish --title "Report" --expires 7d ./report
seol publish --quiet ./report
seol publish --json ./report
seol history
seol list
seol stats
seol info PAGE_ID
seol replace PAGE_ID ./report
seol expiry PAGE_ID 3d
seol delete PAGE_ID</code></pre>
    <p class="note"><code>--quiet</code> prints only the URL. <code>--json</code> returns machine-readable output. Replace <code>PAGE_ID</code> with an ID from your publish output or page list.</p>
    <p class="note"><code>history</code> shows local publishing metadata and source paths; it does not restore expired content. <code>expiry PAGE_ID 3d</code> sets expiry to three days from now. Use <code>info PAGE_ID</code> to check the date.</p>
  </section>

  <section id="agent-handoff" aria-labelledby="agent-handoff-title">
    <div class="handoff">
      <div>
        <h2 id="agent-handoff-title" tabindex="-1">Give this to your agent</h2>
        <p>Install and configure Seol first. Then use this request to create a page and return a shareable link.</p>
      </div>
      <div class="snippet">
        <div class="snippet-label"><span id="handoff-label">Request to copy</span><button type="button" data-copy="handoff-command" aria-label="Copy agent handoff request" hidden>Copy</button></div>
        <pre tabindex="0" role="region" aria-labelledby="handoff-label"><code id="handoff-command">Create a static site for the result.
Publish it with:

  seol publish --quiet DIRECTORY

Return the shareable URL.
Do not include private data.</code></pre>
      </div>
    </div>
    <p class="note">For repeated use, ask Codex to use <code>$skill-installer</code> to install the <a href="https://github.com/semyonfox/seol/tree/main/skills/seol">Seol skill</a>.</p>
  </section>

  <section aria-labelledby="self-host">
    <h2 id="self-host" tabindex="-1">Run your own</h2>
    <p>Seol is one Go binary with SQLite metadata and filesystem storage. The included Compose setup can run it with an optional Cloudflare Tunnel sidecar.</p>
    <pre tabindex="0" role="region" aria-label="Self-hosting commands"><code>git clone https://github.com/semyonfox/seol.git
cd seol
cp .env.example .env
# Set SEOL_TOKEN in .env, then:
docker compose up --build -d</code></pre>
    <p class="note">See the <a href="https://github.com/semyonfox/seol#quick-start">server setup instructions</a> for configuration and tunnel setup.</p>
  </section>
</main>
<footer>
  <a href="https://github.com/semyonfox/seol">Source on GitHub</a> · MIT licensed
  <details id="privacy-settings" data-enabled="{{.TelemetryEnabled}}" data-endpoint="{{.TelemetryEndpoint}}">
    <summary>Privacy</summary>
    <p>Optional self-hosted statistics count landing-page views, command copies and fixed copy-error categories. They never include page content, links, IDs, error messages or visitor identifiers. Counts expire after 30 days and error counts after 14 days.</p>
    <p>Collection is off until the server owner configures it. Browser privacy signals are respected. Your opt-out preference stores only a boolean on this browser.</p>
    <label class="privacy-toggle"><input id="anonymous-counts" type="checkbox" disabled>Allow anonymous counts</label>
    <p id="privacy-status" role="status" aria-live="polite" aria-atomic="true">Anonymous counts are off.</p>
  </details>
</footer>
</body>
</html>`))

func (s *Server) landingPage(w http.ResponseWriter, _ *http.Request) {
	var page bytes.Buffer
	endpoint := ""
	if s.cfg.TelemetryEnabled {
		endpoint = s.cfg.TelemetryEndpoint
	}
	if err := landingPageTemplate.Execute(&page, struct {
		PublicBaseURL     string
		TelemetryEnabled  bool
		TelemetryEndpoint string
	}{s.cfg.PublicBaseURL, s.cfg.TelemetryEnabled, endpoint}); err != nil {
		http.Error(w, "Could not render landing page.", http.StatusInternalServerError)
		return
	}
	// The public landing page can be indexed; temporary uploads retain noindex.
	w.Header().Del("X-Robots-Tag")
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

func serveLandingScript(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write(assets.LandingJS)
}
