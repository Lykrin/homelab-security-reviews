# Security reviews of self-hosted systems

Browse these articles at [Lykrin Security Research](https://lykrin.github.io/).

Source-level reviews, deployment hardening, identity policy, least-privilege controls and recovery security. Each article identifies its evidence, enforcement boundary and practical repair or review method.

| Case study | Work / review date |
|---|---|
| [Closing an intake's outbound trust-boundary gap](case-studies/support-intake-egress-isolation.md) | 26 September 2026 |
| [Glance: TLS, redirects, and the browser boundary](case-studies/dashboard-credential-transport.md) | 26 September 2026 |
| [Authentik discovery is not a client's grant allowlist](case-studies/authentik-grant-policy.md) | 26 September 2026 |
| [Authorize the VPN target before invoking its backend](case-studies/vpn-control-plane-authorization.md) | 5 July 2026 |
| [SSH adapters and remote shell boundaries](case-studies/vpn-remote-command-boundaries.md) | 30 September 2026 |
| [VPN revocation and rollback state](case-studies/vpn-revoke-rollback-state.md) | 5 July 2026 |
| [A backup contract failed safely while its health display stayed green](case-studies/backup-contract-observability.md) | 26 September 2026 |
| [Encrypted backups need a key path that survives the outage](case-studies/encrypted-recovery-bootstrap.md) | 26 September 2026 |
| [Native identity, local fallback, and narrow API exceptions](case-studies/native-identity-and-api-exceptions.md) | 26 September 2026 |
| [Forgejo enrollment: OpenID is not OpenID Connect](case-studies/forgejo-external-provisioning-policy.md) | 26 September 2026 |
| [Closing the direct-backend route around a login gateway](case-studies/proxy-backend-firewall-contracts.md) | 31 August 2026 |
| [Turning MQTT credentials into per-role topic boundaries](case-studies/mqtt-topic-authorization.md) | 31 August 2026 |
| [Glance configuration controls a privileged network client](case-studies/glance-request-destination-authority.md) | 30 September 2026 |
| [Where Glance API strings acquire markup authority](case-studies/glance-active-content-boundary.md) | 30 September 2026 |

Dates refer to recorded original work or the actual source review. Articles were prepared on 30 September 2026, with later analysis identified where relevant. Private evidence remains outside this repository. Research dates describe the work, not earlier public publication.

## Original local probes

Three standard-library Go examples accompany the Glance articles:

- [Redirect headers and origin policy](examples/glance-redirect-probe.go)
- [Request destinations and route policy](examples/glance-destination-policy-probe.go)
- [Contextual escaping and trusted content](examples/glance-template-boundary-probe.go)

Their recorded runs passed. Request probes use synthetic hostnames mapped directly to loopback listeners; the template probe stays in memory. They illustrate specific library boundaries rather than execute Glance.

```fish
run-bounded 300 go run examples/glance-destination-policy-probe.go
```

Requires Go. `run-bounded` is the author's local deadline wrapper; another system can use its timeout tool.
