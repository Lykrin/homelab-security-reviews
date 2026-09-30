# VPN revocation: removing access and removing a row are different operations

**A peer disappears from a dashboard when metadata is deleted; it loses VPN access when the live interface stops accepting its key.** A source review must follow both. This case reconstructs a July control-plane fix, its ordering regression test and a subtle remaining contract: rollback after a bookkeeping failure can restore a peer that was briefly revoked.

## Identify all four states

The reviewed adapter manages four representations of a peer:

1. A persistent server configuration containing the peer's public key and tunnel-address permission.
2. The running VPN interface, changed by a configured apply command.
3. A client table used for names and display metadata.
4. A local peer registry used by the control plane to resolve names and addresses.

Deleting only the display row leaves the first two intact. Editing only the persistent file leaves a running interface unchanged until configuration is applied. Removing a local registry record first can make a still-live peer harder to locate through the controller.

WireGuard's upstream manual distinguishes persistent configuration input from commands that set live interface state. Its `syncconf` operation compares the provided configuration against the interface and applies differences. This explains why an apply step is security-relevant, but it does not establish the contents or success of this application's administrator-configured command. The AmneziaWG deployment was not exercised here. [WireGuard command semantics](https://git.zx2c4.com/wireguard-tools/plain/src/man/wg.8).

## Reconstruct the successful revoke

The adapter resolves the requested name or address against its registry and selects the stored public key. It then takes remote backups. Inside the remote mutation routine it removes that key's block from the persistent configuration, applies the configuration, and only afterwards removes the corresponding client-table entry. The local registry is updated after the remote routine returns.

```text
Resolve known peer → back up remote files
                   → write configuration without peer
                   → apply to live interface
                   → remove client-table row
                   → save local registry without peer
```

The July ordering test records calls to mocked transport methods. It asserts that `apply()` appears before the table write, and that the saved registry is empty after the mocked successful revoke. Another existing test strips the final peer while retaining the interface section. These protect two concrete properties: access withdrawal precedes cosmetic bookkeeping, and revoking the last peer must not destroy the interface configuration.

They do not prove that packets were blocked on a real interface. The apply call is mocked, and the test's transport data is synthetic. For this write-up the tests were inspected, not executed; no new test pass or live revocation timing is claimed.

## The historical rollback defect

A July 5 patch changed the earlier restore routine from direct copying over destination files to the same guarded temporary-write-and-move path used for normal changes. It also introduced a rollback helper that preserves the original mutation error on successful recovery and reports both mutation and recovery failures when rollback fails.

That change addresses a real asymmetry in the old implementation. A failure-recovery path should not bypass the file replacement safeguards that protect the normal operation. If a direct copy is interrupted after truncating its destination, the recovery attempt itself can damage the configuration it is supposed to restore.

The current writer sends encoded content into a neighboring temporary file and moves it into place only after the write command succeeds. For configuration writes it also rejects content unless exactly one interface-section marker is present. The guard is a narrow sanity check, not a full configuration parser. It cannot establish valid peer keys, address permissions or all backend-specific options.

Python's filesystem documentation describes successful rename/replace as atomic on POSIX and notes the cross-filesystem constraint. This supports the single-file replacement model. It does not make two file replacements and a live interface update one atomic transaction, and it does not establish crash durability without inspecting flush and filesystem behavior. [Python replacement semantics](https://docs.python.org/3/library/os.html#os.replace).

## Failure points define the contract

The important behavior emerges after a successful live apply but before bookkeeping completes. If the client-table write fails, the exception handler restores the backups and applies them. If that recovery succeeds, the original peer is reintroduced and the revoke call fails. The operation is a compensating transaction with an attempt to return to its prior state; it is not a guarantee that a requested emergency revocation stays in force despite metadata failure.

| Failure point | State supported by source analysis | Caller result |
|---|---|---|
| Peer not found in local registry | No remote mutation attempted | Lookup error |
| Backup creation fails | Mutation routine has not started writing | Backup error propagates |
| New configuration write fails | Recovery attempts prior files and applies them | Original error if recovery succeeds |
| Live apply fails | Disk may contain new config; recovery attempts prior state | Original error or explicit recovery failure |
| Table write fails after successful revoke apply | Peer was removed live; successful rollback can restore it | Revoke fails with original error |
| Restore or recovery apply fails | Disk, live interface and metadata may diverge | Explicit rollback-failure exception with backup reference |
| Local registry save fails after remote routine succeeds | Remote revoke succeeded; local registry can remain stale or partial | Save error propagates |

These are control-flow deductions, not a fault-injection trace. The final row is outside the remote routine's rollback block. The local registry writer opens its file for replacement contents directly; it does not participate in the remote backup transaction. A web error response after that point does not imply that access remained enabled.

## An original pattern for documenting the transaction

This generalized pseudocode expresses the inspected recovery policy. It illustrates how to make failure states explicit; it is not copied private source and has not been applied.

```python
def revoke_transaction(peer):
    snapshot = backup_remote_state()
    try:
        replace_remote_config(remove_peer(peer.public_key))
        apply_remote_config()  # live access withdrawal
        replace_remote_metadata(remove_row(peer.public_key))
    except Exception as original:
        try:
            restore_remote_state(snapshot)
            apply_remote_config()  # may restore peer access
        except Exception as recovery:
            raise StateDivergence(snapshot, original, recovery) from original
        raise

    save_local_registry_without(peer)  # separate failure domain
```

An emergency-disable endpoint may need a different contract: after a confirmed live removal, a display-table failure could be reported as incomplete bookkeeping without deliberately restoring access. That is a proposed product and security decision, not an automatic improvement to apply blindly. It requires explicit states and reconciliation behavior, so that an operator can distinguish “access removed, metadata pending” from “revocation rolled back” and “live state unknown.” The existing transactional behavior should be described accurately before selecting another policy.

## Preserve evidence when recovery also fails

The rollback helper rethrows the original exception after a clean restore/apply. If recovery fails, it raises a new exception that identifies the backup timestamp, warns about live/disk divergence and chains the original error. Existing tests inspect this error message and exception cause; another test forces an add-time write failure and observes the backup chosen for restoration.

This is useful incident information, not evidence that restoration completed. A named backup can be missing, damaged or incompatible with later changes; validation requires reading the right authorized files and live state. Operator-only diagnostics should preserve the backup reference and error chain. Public error pages should not expose topology, paths or secrets captured in a remote command's stderr.

The temporary filename and second-resolution backup naming also do not establish serialized writers. Concurrent mutations and crash durability were not tested in this review. Claiming a fully atomic transaction or a guaranteed final state would exceed the inspected evidence.

## A focused maintainer review

For another self-hosted access controller, begin with the operation that changes live authorization, then map its surrounding file and metadata writes. Record which failures compensate, which cannot, and which restore access. Reuse a call-recorder test for the normal ordering, but add fault injection only at meaningful transition points: after live removal, during restore and after remote success before local persistence. Assert the intended final state and error classification, not only HTTP status.

If a patch introduces local atomic replacement, review its file permissions, same-filesystem temporary path and write serialization. If it changes the emergency-revoke policy, test that bookkeeping failures cannot silently convert a denied peer back to an admitted one. Neither proposal was executed against the lab for this article.

The demonstrated historical contribution is the restore-path hardening and explicit recovery-failure reporting, backed by inspected commits and regression tests. The September analysis adds the multi-state model and identifies the compensating-revocation tradeoff.
