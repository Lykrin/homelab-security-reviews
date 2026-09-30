# VPN control planes: authorize the target before touching the backend

**Adding a second VPN backend changes an authorization problem, not just a routing problem.** A user permitted to manage an isolated VPN must not inherit the ability to create or revoke peers on a trusted network because both operations share the same authenticated web application. This source review reconstructs the July hardening of a small Flask control plane and explains the boundary that makes the design work.

## The useful question

The application delegates privileged peer operations to backend adapters. Its initial web routes selected a single isolated backend internally. The July expansion introduced backend names into mutation URLs and added both isolated and trusted planes.

The security question is therefore: **can a caller change the URL's backend name and use the application's authority against a plane that caller cannot manage?** If authentication were the only check, a legitimate user of the isolated plane could potentially select the trusted one. Hiding its tab would not prevent a direct POST.

The historical diff does not establish that a vulnerable multi-backend release existed. The earlier routes addressed one fixed backend; the expansion added backend selection and route-level authorization together. The supported finding is a concrete hardening design that prevents a confused-deputy failure during this expansion, not a demonstrated exploit of a shipped upstream product.

## Reconstruct the two boundaries

The first boundary is **proxy to application**. A Flask `before_request` hook requires a nonempty configured proxy secret, compares the supplied value with `hmac.compare_digest`, and separately requires a username header. A username alone fails; a correct proxy secret without a username also fails. Flask documents app-level request hooks as running before request handling; Python documents `compare_digest` as the comparison primitive intended to reduce timing leakage. These explain the mechanisms, not the state of the deployed proxy. [Flask request hooks](https://flask.palletsprojects.com/en/stable/api/#flask.Flask.before_request), [Python comparison documentation](https://docs.python.org/3/library/hmac.html#hmac.compare_digest).

The second boundary is **authenticated user to selected backend**. Both add and revoke routes first resolve the URL's backend name against the server's configured backend map. They derive its plane from that configuration and check the user against that plane's allowlist. Only then do they parse operation-specific inputs and call the backend command.

```text
Untrusted browser request
  → proxy-origin check + asserted identity
  → configured backend lookup
  → backend's server-owned plane
  → user membership for that plane
  → operation inputs
  → privileged backend mutation
```

That order matters. The browser supplies a backend selector, but never supplies the authoritative plane. The server owns the backend-to-plane mapping. Unknown backends return 404 after the authentication hook; known backends outside the user's plane permissions return 403 before any mutation call.

The listing path uses the same membership predicate to select visible planes and lists only their backends. This reduces accidental disclosure in the rendered index. It complements the POST checks; the actual authorization decision remains in each mutating route. This matches OWASP's recommendation to deny by default and validate authorization on each request. [OWASP authorization guidance](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html).

## A patch pattern worth reusing

The following is original pseudocode illustrating the inspected ordering. It is not copied private application source or a ready-made authentication library. The identity is accepted only under a correctly controlled proxy boundary.

```python
def authorize_target(request, configured_backends, grants, proxy_secret):
    if not proxy_secret or not constant_time_equal(
        request.proxy_key, proxy_secret
    ):
        reject(401)
    principal = request.proxy_asserted_username
    if not principal:
        reject(401)

    target = configured_backends.get(request.backend_name)
    if target is None:
        reject(404)
    if principal not in grants.get(target.plane, []):
        reject(403)
    return target


target = authorize_target(request, backends, grants, proxy_secret)
# Validate add/revoke inputs next; only then perform the mutation.
mutate(target, validated_inputs)
```

Do not authorize a plane name taken from a hidden form field, query string or browser tab. Do not validate the user's access after calling the adapter. A forbidden response sent after a side effect still leaves a created peer or revoked connection.

## The allow/reject contract

This matrix describes the inspected handlers. The historical tests cover the cases identified in the evidence column; other rows are source deductions, not new executed results. All identities below are generalized roles.

| Request condition | Handler outcome | Mutation reachable? | Evidence |
|---|---|---|---|
| Username present, proxy key missing or wrong | 401 | No | Existing rejection tests |
| Correct key, username missing | 401 | No | July username-guard regression test |
| Correct key and identity, unknown backend | 404 | No | Route ordering |
| Isolated-plane operator, trusted-plane add | 403 | No | Existing test also asserts zero backend calls |
| Isolated-plane operator, trusted-plane revoke | 403 | No | Existing test also asserts zero backend calls |
| Trusted-plane operator, valid trusted add | 200 with stubbed result | Yes, stub only | Existing allow-path test |
| Authorized revoke, missing confirmation | 400 | No | Existing test asserts zero calls |
| Authenticated identity absent from all plane lists | Empty authorized index; mutations denied | No | Membership and rendering code |
| Missing policy file | Empty grants; mutations denied | No | Loader's missing-file branch |

The confirmation field on revoke prevents an incomplete request from reaching the backend, but it is not a substitute for authorization or CSRF protection. Likewise, a 200 from an add test with a mocked backend establishes routing behavior; it does not prove real VPN provisioning or network access.

## What the historical evidence actually establishes

Three July 5 commits separate the work into reviewable units: a regression check for the username half of the proxy guard; a per-plane membership module; and backend-aware routes with enforcement and tests. The tests for forbidden trusted-plane add and revoke record attempted backend calls and assert the list remains empty. That is stronger evidence of the intended boundary than merely asserting a 403 response.

The source was inspected statically for this article. Those existing tests were read, not rerun; no fresh pass result or historical execution transcript is claimed. Some web tests rely on policy and backend configuration loaded at import time, while the index visibility test explicitly supplies synthetic grants. The collection should not be described as a hermetic, configuration-independent proof of the whole system.

Missing allowlist data has a useful default-deny property: an unknown plane has no members, and an absent policy file produces an empty map. Other malformed policy errors are not silently converted into success. However, the module loads the allowlist once into process memory. Removing a user from the file does not, by itself, establish immediate revocation in already-running workers. Reload behavior is outside these tests and must be verified separately before promising a revocation deadline.

## Where the boundary still depends on deployment

The proxy secret and username are not two independent user-authentication factors. The application trusts the username because the request is expected to arrive through a trusted proxy that authenticated the user and controls these headers. An attacker who can supply both an accepted key and an arbitrary username can impersonate an allowed principal at this boundary.

Authentik's Caddy integration shows authenticated identity headers copied from the forward-auth response before proxying to the application. The application's extra proxy-key contract is local behavior; it is not a guarantee supplied by that documentation. A deployment review must establish that clients cannot inject or preserve their own identity/key values, that the key is protected in transit and configuration, and that all app entry paths observe the same guard. [Authentik Caddy integration](https://docs.goauthentik.io/add-secure-apps/providers/proxy/server_caddy/).

Route-level checks cover authorization and backend side effects. Live proxy header replacement, network exposure and policy reload timing require separate deployment validation.

## Apply the method to another self-hosted control plane

1. Find the function that performs the privileged operation, then trace every web route that can reach it. Include revoke/delete routes and direct requests, not just the visible UI.
2. Identify which selector is caller-controlled and which mapping is server-owned. Check authorization against the resolved resource, not a second caller-provided label.
3. Write the expected rejection matrix before changing code. Separate missing identity, unknown target, forbidden target and invalid operation input.
4. Reuse the project's test client with synthetic identities and a backend call recorder. The critical forbidden assertion is zero side effects; successful calls can use a stub.
5. Review the proxy/header trust contract and policy reload semantics separately. Route-level tests cannot establish network exposure or the time it takes a policy edit to take effect.
