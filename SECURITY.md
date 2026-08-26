# Security

## Reporting vulnerabilities

Please report vulnerabilities privately through GitHub's security advisory
feature rather than opening a public issue.

## Security model

Seol treats every uploaded page as untrusted passive content.
Anyone who possesses a public page URL can view it; unguessable URLs are not a
substitute for authentication.

- Publishing and management require an accountless bearer credential. The
  database stores only a SHA-256-derived publisher identifier for attribution.
- Artifact responses use a CSP sandbox without `allow-scripts` or
  `allow-same-origin`. Inline CSS works, while all JavaScript, external
  connections, forms, framing, workers, popups, navigation, and active embeds
  are blocked. The policy is applied to every artifact response, not only to
  document types, so an unenumerated content type cannot escape it.
- Checkbox and radio inputs are permitted so a page can offer choices through
  the CSS `:checked` pseudo-class. Nothing can be submitted: `<form>`,
  `<button>`, `<select>`, and `<textarea>` are rejected, the policy sets
  `form-action 'none'`, and the sandbox has no `allow-forms` token.
- Artifacts are served with `Cross-Origin-Resource-Policy: cross-origin`. The
  sandbox gives each page an opaque origin, so a same-origin policy would block
  the page's own stylesheets and images. Artifacts are readable by anyone
  holding the unguessable URL, so this grants no additional access.
- Request logs redact page identifiers. A page ID is the capability that grants
  access to the page and must not reach a log.
- Use a dedicated content hostname with no sensitive cookies.
- Never scope API or administration cookies to a parent domain shared with the
  content hostname.
- Keep the bearer token out of uploaded content.
- Do not upload secrets, credentials, or confidential personal information.
- Put the API behind HTTPS and keep the local origin bound to loopback or a
  private container network.

ZIP uploads reject absolute paths, traversal paths, backslashes, symbolic
links, special files, excessive file counts, and excessive extracted sizes.
All HTML files are parsed before activation and uploads containing executable,
interactive, embedding, or redirecting markup are rejected. CSP is still the
browser enforcement boundary rather than the parser alone.

These controls contain what an artifact can do in a visitor's browser; they
cannot determine whether displayed text is truthful. A static page can still
show deceptive instructions, payment details, or impersonated content. Keep
creation private, expiry short, indexing disabled, and remove abusive content.
