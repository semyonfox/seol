# Seol operations handoff

## Service

- Public URL: <https://seol.semyon.ie>
- Local origin: `http://127.0.0.1:8788`
- Application container: `seol`
- Tunnel container: `seol-tunnel`
- Persistent Docker volume: `seol-data`
- Restart policy: `unless-stopped`

The application serves static uploads only. Publishing and management requests
require `SEOL_TOKEN`; the Cloudflare connector uses a
separate tunnel token. Keep both out of the repository and command output.

The checked-in `compose.yaml` is the preferred reproducible deployment. The
current live tunnel was initially started manually with host networking and
forwards to the local origin.

## Routine checks

```bash
curl --fail http://127.0.0.1:8788/health
curl --fail https://seol.semyon.ie/health
docker ps --filter name=seol
docker logs --tail 100 seol
docker logs --tail 100 seol-tunnel
```

With the CLI configured and the API token available:

```bash
seol stats --server https://seol.semyon.ie
seol list --server https://seol.semyon.ie
```

`seol stats --json` is suitable for scripts. It reports active, expired,
and deleted page counts, stored file and byte totals, and the nearest active
expiry.

## Upgrade

Before changing the live service:

1. Run `make check`.
2. Build a versioned image; do not replace the rollback image tag.
3. Preserve `SEOL_TOKEN`, `SEOL_PUBLIC_BASE_URL`, expiry settings, and
   the upload rate/proxy settings and `seol-data:/data` mount.
4. Start the replacement with host port `127.0.0.1:8788` mapped to container
   port `8080`.
5. Verify local health, public health, and an authenticated `seol stats`.
6. Keep the previous stopped container until the new version has settled.

For a clean installation, copy `.env.example` to `.env`, set both tokens and
the public base URL, then use:

```bash
docker compose --profile tunnel up --build -d
```

## Rollback

The Seol v2 migration retains the previous PageDrop application container under
a dated rollback name and leaves the original `pagedrop-data` volume untouched.
Use `docker ps -a` and `docker volume ls` to identify those rollback resources.

To roll back, stop the current `seol`, give it a unique diagnostic name,
rename the selected stopped container to `seol`, and start it. Confirm
`/health` locally and publicly afterward. Do not delete `seol-data`; all
releases share it.

## Backup

The `seol-data` volume contains both the SQLite database and uploaded page
files. Back up the complete volume consistently. The simplest safe procedure is
to stop `seol`, snapshot or copy the volume, and then restart it. The tunnel
may remain running during this short maintenance window.

## CI and management scope

GitHub Actions runs `go vet`, race-enabled tests, formatting checks, and a
Docker build for every push and pull request. Jenkins handles production
deployment from `main`: it repeats those checks, builds a commit-tagged image,
smoke-tests a disposable candidate, deploys with the existing `seol-data`
volume, retains the previous container for rollback, and verifies the public
health endpoint and homepage.

The Jenkins job and its source-controlled pipeline live in the private
`server-stacks` repository. Branches other than `main` receive CI but are not
deployed.

There is deliberately no web admin dashboard. The authenticated CLI provides
the useful management surface without adding sessions, cookies, or another
privileged UI to secure.

## Abuse control

Authenticated publishing is protected by the application limits (10 MiB uploaded,
50 MiB extracted, 100 archive entries, one-day default expiry, seven-day
maximum), a two-upload concurrency cap, and a built-in upload rate limit. Production sets
`SEOL_UPLOADS_PER_MINUTE=5` and `SEOL_TRUST_PROXY_HEADERS=true`, so
Seol groups requests by Cloudflare's validated `CF-Connecting-IP` value.
Enable trusted proxy headers only while the origin remains private behind the
tunnel.

An equivalent Cloudflare rate-limiting rule for `POST /api/v1/pages` is useful
defense in depth when the zone plan supports method matching. Keep public page
reads and authenticated management requests outside that edge rule. The
application limit remains authoritative because Cloudflare rate-limit matching
capabilities vary by plan.
