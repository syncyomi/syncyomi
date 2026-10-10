# Security

Report a vulnerability privately to the maintainer rather than in a public issue: use GitHub's "Report a vulnerability" button on the Security tab if it is enabled, otherwise email the address on the maintainer's GitHub profile.

Only the latest release is supported with fixes; `ghcr.io/syncyomi/syncyomi:latest` tracks it. Every pull request runs `govulncheck`, and the published image is scanned daily with Trivy.
