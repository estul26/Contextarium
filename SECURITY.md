# Security Policy

Contextarium is intended to store sensitive user-controlled context.

Baseline requirements:

- Never commit credentials, tokens, private keys, or real user data.
- Store credential verifiers/hashes, not plaintext credentials.
- Supply runtime secrets through environment/runtime configuration.
- Use parameterized SQL only.
- Enforce authorization in application services before protected record access or mutation.
- Enforce least-privilege scopes and namespace restrictions.
- Use revision-aware conflict checks for authoritative mutations.
- Append required audit events for security- and state-relevant operations.
- Bound request, response, query, validation, and import work.
- Authenticated traffic crossing an untrusted network must use encrypted transport.
- Bearer credentials must not be intentionally transmitted in plaintext across an untrusted network.
- Sensitive backups stored remotely must be encrypted before leaving the trusted host boundary.

A loopback-only development deployment may use local HTTP. Remote exposure before the relevant authentication/security milestones is unsupported.

No production release exists yet. Until a private disclosure channel is configured, do not post working exploits or sensitive data in public issues.
