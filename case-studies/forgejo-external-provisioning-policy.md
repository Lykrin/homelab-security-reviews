# Forgejo enrollment policy: OpenID is not OpenID Connect

## Closed local registration does not settle external enrollment

The September identity review asked a question that applies to any Forgejo instance using an external identity provider: can a new external identity create an application account even though the local signup form is closed? The useful outcome was tracing separate enrollment decisions rather than inferring them from the visible login page.

The source expansion changes the answer's precision. **`ENABLE_OPENID_SIGNUP` does not establish the OIDC account-creation policy.** Forgejo has separate legacy OpenID and OAuth2/OIDC code paths. A configuration observation that omits the effective OAuth2 auto-registration value cannot establish whether new externally authenticated identities may obtain accounts.

This distinction matters to both directions of a review. Assuming enrollment is open exaggerates the evidence; assuming `DISABLE_REGISTRATION` closes every callback misses a separate source branch that must be checked.

Public homepage responses and denied unauthenticated user APIs answer a different question from account creation. They can distinguish public reads from protected user operations, but they do not exercise an enrollment callback.

## Trace the actual account-creation path

I pinned the upstream **v15.0.9** tag to commit `19b9b9d216bbfb501c18514bd1a8c980246ca3f7` and followed each enrollment route to its decision.

The [legacy OpenID handler](https://codeberg.org/forgejo/forgejo/src/commit/19b9b9d216bbfb501c18514bd1a8c980246ca3f7/routers/web/auth/openid.go#L228) consults `EnableOpenIDSignUp` and `AllowOnlyInternalRegistration`. It concerns an OpenID URI login, distinct from an OAuth2/OIDC source. If enabled, that route is a separate enrollment surface.

The [OAuth2 callback](https://codeberg.org/forgejo/forgejo/src/commit/19b9b9d216bbfb501c18514bd1a8c980246ca3f7/routers/web/auth/oauth.go#L1076) first resolves the authenticated external user. For a new identity, automatic account creation is selected when internal-only registration is false and `OAuth2Client.EnableAutoRegistration` is true. That branch does not consult `DisableRegistration`. It requires a provider user ID and sufficient profile fields, sets the external source relationship, derives account flags, then calls the shared creation helper.

The [OAuth2 settings loader](https://codeberg.org/forgejo/forgejo/src/commit/19b9b9d216bbfb501c18514bd1a8c980246ca3f7/modules/setting/oauth2.go#L69) reads `ENABLE_AUTO_REGISTRATION` from the `oauth2_client` section. The setting defaults false when unset. Default behavior is useful context, but does not replace inspecting the effective deployed value and overrides.

```text
ordinary form → DisableRegistration gate → local account creation
legacy OpenID → EnableOpenIDSignUp gate → its enrollment path
OAuth2/OIDC callback → existing external identity?
  yes → application sign-in
  no  → auto-registration enabled and internal-only disabled?
          yes → automatic creation path
          no  → account-linking page
```

The fallback account-linking flow has its own checks: [the registration handler](https://codeberg.org/forgejo/forgejo/src/commit/19b9b9d216bbfb501c18514bd1a8c980246ca3f7/routers/web/auth/linkaccount.go#L217) rejects new registration when ordinary registration is disabled or internal-only registration is enabled. Automatic provisioning and form-based external enrollment therefore need separate validation cases.

## Account linking is an authorization decision

The shared [creation helper](https://codeberg.org/forgejo/forgejo/src/commit/19b9b9d216bbfb501c18514bd1a8c980246ca3f7/routers/web/auth/auth.go#L521) also handles name and email collisions. With automatic linking configured, a collision can lead to linking the external identity to an existing account. With login-based linking, the application instead asks the user to prove the existing account.

That makes profile attributes part of the trust decision. A matching display name is weaker than a stable provider subject. An email value is not sufficient without an issuer whose email claims and ownership lifecycle are trusted. Changing providers or claim mappings can affect identity resolution without changing the visible local username.

The [Forgejo v15 configuration reference](https://forgejo.org/docs/v15.0/admin/config-cheat-sheet/#oauth2-client-oauth2_client) lists linking modes and warns about automatic linking. It also documents independent browser and HTTP Basic authentication settings. Closing enrollment should preserve existing operator access, required Git transport, and deliberate public repository reads.

An enrollment policy therefore needs answers to three separate questions: may this external identity log in, may it obtain a new local account, and may it attach to an existing account? Team membership and administrator privileges are another step, rather than an automatic consequence of authentication.

## A bounded existing-users-only configuration

For an instance whose owner provisions accounts deliberately, the proposed baseline makes each enrollment path explicit:

```ini
[service]
DISABLE_REGISTRATION = true
REQUIRE_SIGNIN_VIEW = false

[openid]
ENABLE_OPENID_SIGNIN = false
ENABLE_OPENID_SIGNUP = false

[oauth2_client]
ENABLE_AUTO_REGISTRATION = false
ACCOUNT_LINKING = login
```

Disabling legacy OpenID is appropriate only when no operator depends on it. The OAuth2 source remains enabled so existing linked users can continue signing in. `REQUIRE_SIGNIN_VIEW=false` preserves the separate public-read decision; making repositories private or requiring login is a different policy change.

For a team that wants automatic provisioning, first restrict admission at the identity provider, then inspect the source's required-claim and privilege mappings. A broad group claim used for admission must not accidentally map to administrator privileges. Explicitly test the admitted group's account defaults before enabling automatic creation. This is an illustrative closed-enrollment policy; the review did not apply an enrollment change.

## Verify enrollment without risking the operator account

1. Record the effective values of both registration sections, the source type, account-linking mode, required-claim settings, and group-to-privilege mappings. Record presence and policy values without exporting client secrets or user membership.
2. Use a restored, isolated instance and separate provider test identities. Confirm an already linked operator can log in, an unlinked identity cannot create an account under the closed policy, and account count remains unchanged after denial.
3. Exercise the ordinary form, legacy OpenID route, and OAuth2 callback separately. A hidden signup button is not a request-level denial.
4. Test a deliberate name/email collision with a nonprivileged test account. Require proof of that existing account instead of treating profile equality as ownership.
5. Verify the homepage, intended public repository clone, protected user API, and required authenticated Git operations retain their expected boundaries.

Transport deserves a separate review: a directly reachable application listener can avoid proxy TLS and header policy even when native authentication remains intact. Restrict a protected listener to its intended ingress and machine consumers while preserving required Git transport. Review enrollment and transport changes separately so a callback failure is not confused with network denial.

Save the prior configuration and source metadata before an approved change. Forgejo configuration changes require a full restart; schedule that interruption, validate the allowed operator first, and restore the prior configuration if existing identity linkage fails. Rolling back configuration does not automatically remove a test account or unlink a new identity, which is another reason to validate creation in an isolated restoration.

The original review date is **26 September 2026**. Expanded source analysis: **30 September 2026**. Effective OAuth2 auto-registration remains a required observation in any deployment-specific validation.
