# Closing the direct-backend route around a login gateway

A reverse proxy can require login while its backend remains reachable through a different network route. The August policy change added source-specific permits followed by explicit drops on selected backend listeners. The resulting contract was precise: proxy traffic and named machine consumers could reach those listeners; other network peers could not take the direct route. A reusable rule-ordering example below illustrates why listener-specific drops must be evaluated with the surrounding defaults.

This review reconstructs the historical contract using sanitized firewall additions and application access descriptions. The example is not an inventory of current infrastructure. It separates transport admission, user authorization, and health access rather than treating a login redirect as proof of isolation.

## The bypass path and its three decisions

Forward authentication delegates an authorization check to an identity outpost while the existing reverse proxy carries application traffic. It does not automatically insert that check into a direct connection to the application. That separation is explicit in [Authentik's forward-auth documentation](https://docs.goauthentik.io/add-secure-apps/providers/proxy/forward_auth/).

The relevant threat path is therefore: a network peer learns a backend listener, connects without traversing the proxy, then supplies an application request. An application with no native login may accept the request directly. An application that trusts an identity header without authenticating its sender may instead accept a fabricated identity. Neither possibility requires the gateway's own login mechanism to fail.

The retained design addressed three different questions:

1. **Can this source reach the backend listener?** Guest firewall rules admitted the proxy and selected machine consumers, then dropped other sources for that same interface, transport, and destination listener.
2. **Can this caller exercise a user route?** Two custom applications independently required an identity header and checked that the actual TCP source was the designated proxy. A header alone was insufficient.
3. **Can this caller obtain liveness information?** Health endpoints used separate source allowlists. Their machine callers were not converted into interactive users by the firewall exception.

The second decision is application-specific. Other applications in the retained record relied on the proxy or their own login. It would overstate the evidence to attribute the custom peer check to every protected service.

## What the policy addition establishes

The historical change placed source permits before listener-specific drops. The reusable example below chooses permissive defaults solely to make that ordering requirement visible; it does not reproduce the guest-wide policy from the reviewed environment.

| Historical rule shape or worked-example property | Supported conclusion |
|---|---|
| Matching-source permits precede an explicit drop for each protected listener | The intended restriction applies to that listener, including otherwise permitted local-network sources |
| The illustrative example uses permissive defaults | Listener-specific rules must be evaluated separately from guest-wide isolation |
| Custom application descriptions require identity plus proxy TCP source | Network admission and user authorization are separate decisions |
| Health consumers are named separately | Machine liveness access is an explicit exception needing its own application boundary |
| The historical gateway record reports unauthenticated roots redirecting to login | The proxy gate was recorded as active; that observation alone does not prove every backend route was denied |

Proxmox stores guest policy in a guest-specific firewall file and supports interface, source, transport, destination-port, and IP-set selectors. It also requires the virtual interface's firewall flag in addition to the general enable option. Those platform requirements explain why a correct-looking source file is insufficient evidence of active enforcement. [Official Proxmox firewall documentation source](https://github.com/proxmox/pve-docs/blob/master/pve-firewall.adoc)

## Generalized policy diff

This original reusable example illustrates source permits followed by a listener-specific drop. Its defaults and source roles are illustrative choices, not installed settings. `APP_LISTENER` must be replaced with a destination-port value, and the named source sets must be defined for the reader's own lab. The excerpt is not deployable as written.

```diff
--- /dev/null
+++ backend-policy.fw
+[OPTIONS]
+enable: 1
+policy_in: ACCEPT
+policy_out: ACCEPT
+
+[RULES]
+IN ACCEPT -i net0 -source +loginproxy -p tcp -dport APP_LISTENER -log nolog
+IN ACCEPT -i net0 -source +healthconsumer -p tcp -dport APP_LISTENER -log nolog
+IN ACCEPT -i net0 -source +dashboardconsumer -p tcp -dport APP_LISTENER -log nolog
+IN DROP -i net0 -p tcp -dport APP_LISTENER -log info
```

The final drop is essential under the example's permissive default. Merely adding proxy and monitor permits would not exclude everyone else. Conversely, placing the drop before those exceptions would break their intended path. Repeating the block for another listener creates another specific contract; it does not silently restrict unrelated listeners or guest-initiated traffic.

## Positive and denied paths

The following matrix expresses the reusable example and the application authorization checks described above. It is not a new runtime test result.

| Path | Transport policy | User-route decision in the two custom applications |
|---|---|---|
| Proxy to protected listener with verified identity | Permit | Eligible for application authorization |
| Proxy to protected listener without identity | Permit | Reject user request |
| Approved monitor to designated health route | Permit | Separate health allowlist applies |
| Approved monitor to user route with fabricated identity | Permit | Reject: TCP source is not the proxy |
| Ordinary peer directly to protected listener | Drop | Application should not receive the connection |
| Traffic to an unrelated listener | Not decided by this block | Requires its own review |

## Review procedure for an authorized lab

1. Trace one request from client to proxy, identity check, and backend. Record where the user identity is established and where the backend consumes it.
2. Enumerate that backend's actual listeners and interfaces from existing configuration. Include secondary APIs; reviewing only the browser route can miss a second transport entry point.
3. Assign every permitted source a role. Distinguish user proxying from machine health and data-fetch traffic; specify the application paths each machine consumer needs.
4. Review the guest rule sequence and defaults together. Check cluster activation, guest enablement, interface flags, inherited policy, and the generated rules for the deployed backend in use. A repository policy addition proves intent, not those deployment prerequisites.
5. With separate authorization for testing, exercise both required and forbidden paths using disposable data. Verify a real application response on the proxy route, a narrowly useful health response, and direct-route denial. Preserve the source class and response meaning, not private addresses or identity values.

## Scope

The August artifact records implemented listener restrictions and application access contracts. This September write-up reviewed their historical source and supporting documentation; it did not replay traffic or establish current runtime state. Source-address admission depends on the network's resistance to impersonation and on the permitted hosts remaining trustworthy. It does not provide cryptographic service identity, protect unrelated listeners, or constrain egress.
