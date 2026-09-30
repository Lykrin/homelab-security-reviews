# Authentik grant policy: discovery is not a client's allowlist

**Authentik 2026.8.3 advertises a fixed discovery grant list but enforces grants against each provider's stored configuration.** A discovery-only review can wrongly label a disabled flow as enabled—and miss a supported flow absent from that list. Tracing both request paths identifies the actual enforcement points.

## Version and question

The source-review baseline is **Authentik 2026.8.3**, whose upstream release resolves to commit `e5a0d2f7572cb776eee7a3e9355937ce38973761`; the code links below use that immutable revision. The documentation links use the 2026.8 release series, which can evolve independently of this patch release. [Release 2026.8.3](https://github.com/goauthentik/authentik/releases/tag/version%2F2026.8.3), [release commit](https://github.com/goauthentik/authentik/commit/e5a0d2f7572cb776eee7a3e9355937ce38973761).

The question was precise: **does seeing a grant in an application's discovery response establish that its client may use it?** The OpenID Connect specification describes `grant_types_supported` as authorization-server support metadata. Authentik separately documents provider-configurable grant selection. Neither source, alone, identifies how a request reaches the stored policy. [OIDC Discovery, provider metadata](https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderMetadata), [Authentik 2026.8 provider configuration](https://version-2026-8.goauthentik.io/add-secure-apps/providers/oauth2/#oauth-20-flows-and-grant-types).

## Follow the discovery response to its producer

In `authentik/providers/oauth2/views/provider.py`, `ProviderInfoView.dispatch()` resolves the application slug and its provider. That makes the URL look like a provider-specific configuration export. However, `get_info()` fills `grant_types_supported` from a fixed list of constants; it does not read `provider.grant_types` for that field. Other fields do use provider state, including issuer, keys and configured scope mappings. The response mixes these sources. [Pinned discovery implementation](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/views/provider.py).

The inference is field-specific: **a provider-specific URL does not make every returned field a provider-specific authorization decision.** Changing the stored grant selection does not change the list construction shown in this function. That is a source inference, not a reported before/after experiment.

There is a second useful mismatch. The fixed discovery list omits token exchange, while the pinned grant enum and token router include it. Thus omission from this list also cannot establish that the implementation lacks the corresponding route. The discrepancy concerns metadata; actual grant availability depends on each provider's policy. [Discovery list](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/views/provider.py), [grant enum](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/models.py), [token router](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/token/router.py).

## Locate policy storage, then both consumers

`OAuth2Provider` stores `grant_types` as a PostgreSQL `ArrayField` with grant-enum choices. This is per-provider persisted state. The model's `default=list` is not evidence of the UI's creation defaults or an existing provider's contents; those require separate configuration evidence. [Pinned provider model](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/models.py).

The authorization path consumes that state through `OAuthAuthorizationParams.check_grant()`. It maps a browser request's response type to an internal grant, rejects an unresolved response type, then rejects a grant absent from the selected provider's array with `invalid_request`. Browser `response_type` and token-endpoint `grant_type` are distinct inputs; searching only token POST handling would miss this policy consumer. [Pinned authorization checks](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/views/authorize.py).

The token path has a different entry point. `TokenView.post()` extracts client authentication inputs, looks up the provider by client ID and calls `parse_token_request()`. A missing provider yields `invalid_client`. Lookup identifies the policy owner; it does not, by itself, authenticate the caller. [Pinned token view](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/views/token.py).

The router selects a request class for the submitted grant and rejects unknown routes with `unsupported_grant_type`. Each of the five routed request implementations calls the shared parser. `TokenRequest.parse()` checks membership in `self.provider.grant_types` and raises `invalid_grant` with cause `grant_type_not_configured` when membership fails. [Router](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/token/router.py), [shared parser](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/token/base.py), [authorization-code parser](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/token/authorization_code.py), [refresh parser](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/token/refresh_token.py), [client-credentials parser](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/token/client_credentials.py), [device-code parser](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/token/device_code.py), [token-exchange parser](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/token/token_exchange.py).

## The trust boundary

The boundary is where a caller-supplied protocol choice meets server-owned provider policy. Discovery supplies metadata; it is not input to the membership check. This diagram summarizes the inspected paths, rather than a captured execution trace:

```text
Browser response_type → resolved internal grant → provider grant check

Token client inputs → provider lookup → grant request router
                    → shared provider grant check → further validation

Discovery request → application/provider lookup → fixed advertised grant list
```

Passing membership is necessary along these paths, not sufficient for token issuance. The shared parser also performs confidential-client checks for specified flows and scope processing; grant-specific parsers add their own validation. Enabling a grant is therefore not synonymous with unauthenticated token access. [Shared validation](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/token/base.py).

An empty stored grant array would fail these membership checks; treating it as “all grants” would reverse the inspected contract. Likewise, the `password` route maps to the client-credentials request implementation. Its label should not be interpreted as proof of a conventional user-password login path; the matching release documentation describes this compatibility behavior. [Token router](https://github.com/goauthentik/authentik/blob/e5a0d2f7572cb776eee7a3e9355937ce38973761/authentik/providers/oauth2/token/router.py), [2026.8 machine-to-machine documentation](https://version-2026-8.goauthentik.io/add-secure-apps/providers/oauth2/machine_to_machine/).

## Evidence and verdict

| Evidence | Supported conclusion |
|---|---|
| Archived local review inspected selected provider grant fields and recorded a least-privilege advisory | There was a policy-review task; no completed restriction or exercised token flow is claimed |
| Pinned discovery producer uses a fixed list | Its grant metadata is not a copy of a client's stored allowlist |
| Pinned model and two request paths use per-provider grant state | The inspected implementation has an explicit enforcement boundary |
| A grant has a route but is absent from the fixed advertised list | Discovery is insufficient as a complete route/policy inventory |
| Request handlers have further checks after route selection | A supported or configured grant does not establish token issuance |

The security conclusion is a review correction: use provider configuration and its enforcing handlers to assess least privilege. A permissive provider deserves a consumer/dependency review, but permissiveness alone does not prove bypass or compromise.

## Reuse this analysis in an authorized environment

1. Pin the installed release and corresponding source revision. Record any mismatch between upstream code and a locally patched image.
2. Find the producer of the metadata field you intend to use as evidence. Trace whether it is constant, derived from persisted policy or filtered for the current client.
3. Inspect only the relevant provider policy fields through an authorized administrative interface. Keep client secrets, IDs, names and raw configuration exports out of the write-up.
4. Trace every entry point that consumes the policy. Distinguish browser response types, token grants, grant routing, membership rejection and subsequent authentication.
5. Map each consumer's required protocol before proposing restrictions. A browser login, background refresh and machine-to-machine task need different evidence; do not change a provider from its label alone.
6. Prepare a specific configuration diff and rollback. Any later runtime validation should use an approved test client and check both a required flow and a deliberately disabled one. A discovery refresh alone would not validate the enforcement change.

## Validation coverage

The source review covers the pinned release's discovery producer, browser authorization check and token-request routing. OAuth flows were not exercised. Matching a release version does not establish that a locally patched image is byte-identical.
