# Encrypted backups need a key path that survives the outage

The September 26 recovery review found encrypted backup generations alongside a documented protected recovery replica that could not be retrieved. The repair recreated that replica, compared recovered key material with the expected live key, and used the recovered key to decrypt a bounded real restore. The restored bytes matched the source checksum.

Isolated application restores also passed integrity and selected runtime checks. The useful contribution was a stronger chain of proof: encrypted snapshot metadata, recovered-key equality, successful decryption and application-level recovery each received a separate check.

## Encryption metadata is the first layer

[Proxmox's client documentation](https://pbs.proxmox.com/docs/backup-client.html) describes client-side encryption and warns that a lost key makes the corresponding backed-up files inaccessible. The existence of an encrypted snapshot is therefore only the beginning of a recovery argument. A backup checker can confirm encryption mode and key fingerprint without proving that an alternate key path remains usable.

A verification record answers another question. [Proxmox's verification documentation](https://pbs.proxmox.com/docs/maintenance.html#verification) describes scheduled integrity checks and rechecking existing backup data. That evidence is distinct from opening the encrypted payload with a recovered key, importing a database or booting an application.

These distinctions explain an important diagnostic rule: missing verification evidence does not itself establish plaintext storage, corruption or failed decryption. Each claim needs the corresponding observation.

## Model prerequisites before selecting recovery mechanisms

This reusable graph illustrates the dependency to inspect; it is not a map of installed recovery arrangements:

```text
encrypted archive --> usable decryption key --> restored contents
                           ^
                           |
              protected recovery replica
                           ^
                           |
              independent bootstrap assets
```

The arrows represent prerequisites, not automatic recovery steps. An archive cannot independently supply the external key needed to open itself. Likewise, a protected replica is insufficient if the mechanism needed to unlock it depends on the very system being recovered.

For any chosen protection mechanism, review three dependencies separately: the replica's location, the means to unlock it, and the metadata needed to select the matching key. Several named recovery methods can share one failure domain. An independently available bootstrap path should be demonstrated rather than inferred from the number of copies.

## A documented replica was not a working replica

The initial report described a recovery path without a completed decrypt test. The localized investigation established a concrete defect: the documented protected replica could not be retrieved. Working live encryption had concealed a broken alternate recovery path.

The repair recreated the protected replica without changing the live backup key. Recovered material then passed a hash comparison against the expected key. A bounded encrypted restore accepted that recovered key, and the restored bytes matched the source checksum. No plaintext key was persisted.

Those observations prove more than a documentation update. They establish that a retrievable protected replica yielded the intended key and that the key performed a real decryption operation. They still describe bounded validation, rather than a complete host reconstruction.

## A proof ladder for recovery claims

| Layer | Recorded result on September 26 | What it proves |
| --- | --- | --- |
| Encryption identity | Snapshot and expected key metadata agreed | The selected key matched the archive's encryption identity |
| Protected replica recovery | Recovered material matched the expected key hash | The repaired alternate path yielded the intended key |
| Recovered-key operation | A bounded real restore decrypted and matched its source checksum | The recovered key was operationally usable |
| Isolated application restore | Data integrity and selected loopback runtime checks passed | The exercised application data and behavior survived restoration |
| Native application data | Database import and archive extraction passed | Those data-format recovery operations worked |

Verification status and snapshot recency should remain separate selection criteria. The newest generation need not be the newest verified generation. A restore report should identify which property determined its selection rather than calling either one simply “the latest good backup.”

Restore isolation also matters. Historical identities, hooks and addresses can make a recovered application contact live services. The recorded checks used isolated scratch resources and loopback application access; temporary recovery resources were removed afterward.

## A reusable validation sequence

1. Select a disposable recovery target and the encrypted artifact to restore. Record encryption identity, verification status and generation selection separately.
2. Obtain the matching key through the deliberately chosen protected recovery path. Prove key identity without publishing key bytes or their identifying fingerprints.
3. Decrypt a bounded artifact and compare known content or checksums. Then validate the relevant data format: database integrity, archive extraction or application-specific consistency.
4. Exercise selected application behavior in isolation. Keep recovered hooks and credentials from contacting production services.
5. Evaluate full bootstrap and authenticated workflows as additional checks. Loss of ordinary online dependencies, clean-host reconstruction and key rotation deserve their own exercises.

The practical result is a distinction between written encryption policy and demonstrated recovery. Metadata answers what was written, verification checks stored integrity, replica recovery checks an alternate key path, and a real restore checks usability. That separation exposed the missing replica and recorded the successful repair without turning a bounded drill into a claim of complete disaster recovery.
