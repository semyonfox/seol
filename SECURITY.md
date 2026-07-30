# Security

## Reporting vulnerabilities

Please report vulnerabilities privately through GitHub's security advisory
feature rather than opening a public issue.

## Security model

Seol treats every uploaded page and its inline JavaScript as untrusted content.
Anyone who possesses a public page URL can view it; unguessable URLs are not a
substitute for authentication.

- Publishing and management require an accountless bearer credential. The
  database stores only a SHA-256-derived publisher identifier for attribution.
- Artifact responses use `sandbox allow-scripts` without `allow-same-origin`.
  Inline CSS and classic inline JavaScript work, while storage, cookies,
  external scripts, external connections, forms, framing, workers, popups,
  navigation, active embeds, and programmatic clipboard access are blocked.
- Use a dedicated content hostname with no sensitive cookies.
- Never scope API or administration cookies to a parent domain shared with the
  content hostname.
- Keep the bearer token out of uploaded content.
- Do not upload secrets, credentials, or confidential personal information.
- Put the API behind HTTPS and keep the local origin bound to loopback or a
  private container network.

ZIP uploads reject absolute paths, traversal paths, backslashes, symbolic
links, special files, excessive file counts, and excessive extracted sizes.
All HTML files are parsed before activation. Page-local event handlers and
classic inline scripts are allowed; external/module scripts, forms, free-text
and file inputs, embedding, and redirecting markup are rejected. CSP is still
the browser enforcement boundary rather than the parser alone.

These controls contain what an artifact can do in a visitor's browser; they
cannot determine whether displayed text is truthful. A static page can still
show deceptive instructions, payment details, or impersonated content. Keep
creation private, expiry short, indexing disabled, and remove abusive content.
