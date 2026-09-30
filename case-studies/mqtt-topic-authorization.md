# Turning MQTT credentials into per-role topic boundaries

The retained MQTT configuration required authentication, but the August change added a distinct authorization boundary: an ACL file assigning publish and receive permissions by client role. A historical check recorded that one device role could publish within its own namespace and was denied publication into another integration's namespace. This analysis develops a reusable ACL model and explains what that limited check could establish.

A device-role credential need not authorize every application's topics. The reusable ACL below shows both a narrow role boundary and broader integration exceptions that deserve explicit review.

## The trust boundary inside one broker

A TCP connection and valid MQTT credentials identify an admitted client. They do not explain which application messages that identity may publish or receive. If several services share a broker, a compromised low-trust client can potentially fabricate another service's telemetry, publish commands, or observe messages outside its function unless topic authorization restricts it.

The sanitized worked example assigns separate credentials to a device producer, an integration bridge and a central automation consumer. Its role names, topic trees and exception widths are illustrative design choices, rather than installed values. Mosquitto's ACL contract is explicit: a `user` block refers to the authenticated username rather than the MQTT client identifier, and the configured ACL limits access to listed topics. [Official Mosquitto configuration manual](https://mosquitto.org/man/mosquitto-conf-5.html)

That distinction matters during review. Changing a client ID does not move a connection into a different username block. Conversely, reusing one privileged username for unrelated applications erases the role boundary even when those applications have different client IDs.

## Historical evidence and a reusable permission model

The historical change added a tracked ACL file and wired it through `acl_file`. Loading a policy requires readable ownership and appropriate file permissions for both ACL and credential files. These are configuration-load prerequisites, not substitutes for topic authorization.

| Reusable example property or historical validation | Supported conclusion |
|---|---|
| Three username-specific blocks with explicit read/write modes | Different authenticated roles receive different message privileges |
| Device role has read/write only under its own prefix | Its credential does not intentionally authorize sibling namespaces |
| Bridge can read one automation-status topic and write the entire automation tree (used for discovery) | Cross-role communication is deliberate and asymmetric |
| Central role has read/write across all three application trees | Central automation remains a privileged identity |
| Historical QoS 1 check records one permitted publish and one denied cross-role publish | Those two write paths were recorded as validated; receive restrictions were not comprehensively tested |

The bridge exception deserves attention. An exact status-topic read is narrow; write access to the entire automation tree includes future descendants. It supports integration behavior but still grants the bridge influence over that part of the automation namespace. A tighter design would enumerate the actual discovery subtree; that is a recommendation for the example, not a claimed historical remediation.

## Generalized before/after shape

This original configuration illustration uses fictional role and topic names, not the installed broker's values. The before state represents the earlier documented authentication contract; it is not a captured complete runtime file. The after state illustrates ACL wiring and deliberately reviewable exceptions. Paths are placeholders for files owned and readable by the broker in the reader's own deployment.

```diff
 # broker configuration excerpt
 allow_anonymous false
 password_file /PATH/TO/PASSWORD_FILE
+acl_file /PATH/TO/ROLE_ACL
```

```conf
# Generalized role ACL: original reusable example
user device-role
topic readwrite devices/#

user integration-role
topic readwrite integrations/#
topic read automation/status
topic write automation/#

user automation-role
topic readwrite automation/#
topic readwrite integrations/#
topic readwrite devices/#
```

The `#` filter covers the parent topic and all descendants. It therefore grants more than “the current topics listed in a dashboard”: new branches inherit the permission. Topic names are case-sensitive, and leading or trailing separators change their identity. These properties follow section 4.7 of the [OASIS MQTT 3.1.1 specification](https://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.html). They make namespace design part of the authorization boundary.

## Permission matrix, including the exceptions

This table is derived from the illustrative ACL shape. “Read” means receiving messages on matching topics, not permission to invoke an unrelated application API. Only the two publish rows marked historical check have retained validation evidence.

| Role and operation | Policy outcome | Evidence level |
|---|---|---|
| Device publishes in its own tree | Allow | Historical positive check |
| Device publishes in bridge tree | Deny | Historical negative check |
| Device receives from automation tree | Deny | ACL-derived |
| Bridge reads exact automation-status topic | Allow | ACL-derived |
| Bridge reads other automation topics | Deny | ACL-derived |
| Bridge publishes into the entire automation tree (used for discovery) | Allow | ACL-derived intentional exception |
| Automation role publishes or receives in any of the three trees | Allow | ACL-derived privileged role |
| Anonymous connection | Reject under documented configuration | Configuration record |

## Why publish acknowledgements need careful interpretation

The historical note records a QoS 1 allowed/denied comparison without retaining enough protocol detail to reconstruct its diagnostic mechanism. It should remain a recorded outcome, not a claim that a particular acknowledgement proved denial.

For MQTT 3.1.1, a server refusing authorization for a publish must acknowledge according to the normal QoS rules or close the connection; the protocol has no publish-denial reason code. A successful publisher exit or PUBACK therefore cannot by itself demonstrate that an unauthorized message was delivered. The specification makes this distinction explicit in [section 3.3.5](https://docs.oasis-open.org/mqtt/mqtt/v3.1.1/os/mqtt-v3.1.1-os.html). A useful lab check needs broker authorization evidence or a separately authorized observer, not just the sender's success indicator.

## Review procedure for an authorized lab

1. Map each application to its authenticated broker username. Separate client IDs from credentials and identify shared privileged identities before reading ACLs.
2. Build a read/write inventory from application configuration: telemetry, commands, status, discovery, and retained-message use. Name every intentional cross-role edge.
3. Resolve the active listener configuration and ACL loading mechanism. Check included files, file readability, and whether authorization applies globally or per listener. Match the installed Mosquitto version rather than copying a newer documentation recommendation blindly.
4. Expand each wildcard into a permission statement, including its parent and future descendants. Review publish and receive permissions separately; a successful publish test says nothing about subscription isolation.
5. With explicit testing authorization, use disposable topics and credentials to check one needed and one forbidden operation per role and direction. Avoid production command or discovery trees. Record broker-side authorization evidence and observer delivery separately, and account for retained messages when interpreting received data.

## Scope

The original August record supports ACL deployment and two write-path outcomes. This September analysis reviewed policy and protocol contracts without reconnecting clients or probing the broker. It does not establish current listener coverage, encryption, password strength, retained-message cleanup, or complete receive-path enforcement. Official documentation was consulted on the write-up date; its current plugin recommendations are not attributed to the historical deployment. The example's central-role privileges and broad bridge exception illustrate design tradeoffs without identifying installed policy.
