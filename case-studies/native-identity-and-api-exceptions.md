# Native identity, local fallback, and narrow API exceptions

## What the identity review established

A missing forward-auth directive is a useful investigation lead, but it does not identify the component responsible for authentication. The September review used Paperless-ngx and FreshRSS to examine different identity boundaries: native OIDC, local recovery login and client API credentials. The source review below pins upstream versions so that each decision can be reproduced.

The contribution was an identity inventory that distinguished browser authentication, recovery access and application API credentials. Expanding the source review also uncovered an important distinction: disabling automatic social signup is different from forbidding social signup.

| Surface to review | Useful validation | Boundary |
|---|---|---|
| Paperless browser login | Identify native OIDC and local recovery routes | Application owns its session and permissions |
| Paperless enrollment | Exercise ordinary and social signup separately | One closed route does not establish every creation policy |
| Paperless documents API | Confirm invalid credentials receive no documents | API credentials form a separate access plane |
| FreshRSS browser login | Resolve the selected form, OIDC or trusted-header mode | Mode selection must match the intended user mapping |
| FreshRSS Fever | Distinguish an unauthenticated handshake from feed data | A protocol response is not proof of authorization |
| Application listeners | Test intended ingress and deny unintended direct access | Browser identity headers are not the entire perimeter |

These distinctions support a policy review, rather than a claim that one common edge configuration should replace all application authentication.

## Three request flows that must remain distinct

With native Paperless OIDC, the browser reaches the application's login route, the application starts the identity-provider exchange, and the callback establishes an application session. Document permissions remain Paperless permissions. A valid central identity does not automatically mean access to every stored document.

With forward-auth, Caddy asks the outpost to authorize the request before forwarding it. Identity headers become meaningful only when the backend trusts that exact proxy and the proxy controls their values. The [Authentik Caddy integration](https://docs.goauthentik.io/add-secure-apps/providers/proxy/server_caddy/) places the outpost route before the authentication subrequest and application proxy inside an ordered route. Adding that mechanism changes request handling; it is not merely an alternative spelling of native OIDC.

FreshRSS's client APIs form another plane. A mobile reader usually cannot complete an interactive web SSO redirect during synchronization. Exempting a narrowly identified client endpoint from forward-auth can be correct when FreshRSS still authenticates the request. Exempting a whole host removes the browser boundary as well.

```text
browser → reverse proxy → native app login → application session → app permissions
browser → reverse proxy → outpost decision → app request → app permissions
reader  → reverse proxy → exact API route → app API credential → user feed data
```

That separation also explains why an application's health check should receive neither a general login bypass nor an API credential with broad data access.

## Paperless: automatic signup is not permission to sign up

Consider an illustrative configuration with `SOCIALACCOUNT_AUTO_SIGNUP=false`. Upstream Paperless **3.2.1**, pinned to commit `7575d6078227ebdb4cf443f263d53ebc7575aa37`, defines a separate `SOCIALACCOUNT_ALLOW_SIGNUPS` setting with a default of true. Its [settings producer](https://github.com/paperless-ngx/paperless-ngx/blob/7575d6078227ebdb4cf443f263d53ebc7575aa37/src/paperless/settings/__init__.py#L322-L331) makes the distinction explicit.

The [ordinary and social adapters](https://github.com/paperless-ngx/paperless-ngx/blob/7575d6078227ebdb4cf443f263d53ebc7575aa37/src/paperless/adapter.py) consult their respective allow-signup settings. Ordinary signup also has a fresh-install bootstrap exception. In an established instance containing users, that bootstrap exception no longer answers the ordinary enrollment question. The social adapter's decision is independent of that ordinary page.

[django-allauth's configuration reference](https://docs.allauth.org/en/latest/socialaccount/configuration.html) describes automatic signup as an attempt to skip the signup form using provider fields. A false value therefore does not, by itself, prove that an authenticated identity cannot reach an account-creation form. A deployment review must inspect the effective social allow-signup value and exercise that flow separately.

For an existing-users-only policy, this illustrative configuration is explicit about both creation paths:

```ini
PAPERLESS_ACCOUNT_ALLOW_SIGNUPS=false
PAPERLESS_SOCIALACCOUNT_ALLOW_SIGNUPS=false
PAPERLESS_SOCIAL_AUTO_SIGNUP=false
```

This closes enrollment policy; it does not define which existing local identity an OIDC subject should link to. Keep the account-linking procedure separate and verify the intended existing user before changing signup settings.

Local recovery deserves the same precision. [Paperless's authentication documentation](https://docs.paperless-ngx.com/configuration/#authentication-sso) states that disabling regular frontend login does not disable Django admin login or local API credentials. If the objective is central authentication for every interactive administrator, a hidden password form is insufficient. If the objective is recoverability during an identity-provider outage, retain a controlled recovery account and document its reachable surfaces.

## FreshRSS: choose the UI identity and preserve the client protocol

A FreshRSS trusted-proxy setting does not itself select form authentication, OIDC or an external-header identity model. Those choices must be resolved independently, along with the client API requirements.

FreshRSS **1.30.0**, commit `62eb3b405e94dd7bc9f1e9e9c788ee3f69d6ed74`, documents OIDC through Apache's `mod_auth_openidc`. The [versioned OIDC guide](https://github.com/FreshRSS/FreshRSS/blob/62eb3b405e94dd7bc9f1e9e9c788ee3f69d6ed74/docs/en/admins/16_OpenID-Connect.md) explains activation, callback handling, and the username claim. That claim must match the intended existing administrator before switching authentication modes.

Its [access-control guide](https://github.com/FreshRSS/FreshRSS/blob/62eb3b405e94dd7bc9f1e9e9c788ee3f69d6ed74/docs/en/admins/09_AccessControl.md) separately describes trusted external identity headers and automatic HTTP-auth user creation. If using that option, allow only the proxy as a trusted source, replace client-supplied identity headers, and explicitly decide whether new identities may create users.

The [versioned Fever protocol](https://github.com/FreshRSS/FreshRSS/blob/62eb3b405e94dd7bc9f1e9e9c788ee3f69d6ed74/docs/en/developers/06_Fever_API.md) distinguishes an unauthenticated handshake from authenticated data responses. Preserve HTTPS and the application's API credential check even when an API route bypasses browser SSO. Store that credential in the reader's secret configuration, rather than in an exception matcher or access log.

## Change, validation, and rollback contract

1. Record the selected browser identity model, enrollment owner, local recovery path, and each actual reader endpoint. Avoid speculative `/api/*` exceptions when one exact route is sufficient.
2. In an isolated deployment, verify an existing allowed OIDC identity reaches only its intended account, a new identity cannot create an account under the closed policy, and an excluded identity is denied.
3. Verify invalid API credentials disclose no feeds or documents, then validate one intended client using its existing application credential. Check the UI and callback still follow their required authentication flow.
4. Confirm an unapproved network peer cannot reach the application port. For header authentication, prove a caller-supplied username cannot survive proxy replacement.
5. Retain the previous configuration and administrator recovery path. Roll back the identity change if account mapping or client synchronization fails; do not widen API exemptions to compensate.

The review did not apply an identity-policy change. Original review: **26 September 2026**. Expanded source analysis: **30 September 2026**.
