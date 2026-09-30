# A backup contract failed safely while its health display stayed green

A backup watchdog repeatedly failed before evaluating capacity and synchronization freshness. Newly backed-up guests were present in the live sync filters but absent from the authoritative inventory and generated contract. Their encrypted destination copies existed. The immediate security problem was an interrupted detection path and stale health evidence, rather than missing backups.

The September 26 review reconciled inventory, generated recovery policy, installed contracts and live job membership. The next ordinary scheduled watcher run exited successfully and reported matching policy. This case follows the failure through the script and separates the repair from a further improvement: representing monitor failure independently of the last successful backup assessment.

## The contract protected a recovery boundary

A backup mirror has several independent properties: intended group membership, encryption selection, retained history and source-deletion behavior must each match declared recovery intent. A successful transfer alone cannot establish those properties.

The reviewed watcher compared effective job membership against its generated contract. For a reusable contract design, encryption selection, retained-generation policy and source-deletion behavior are additional independent fields; the appropriate values depend on the recovery objective.

[Proxmox's sync documentation](https://pbs.proxmox.com/docs/managing-remotes.html) distinguishes group filters, encryption selection and `remove-vanished`: these settings control which snapshots are copied and whether source disappearance removes destination history. They therefore deserve comparison against declared recovery intent, beyond checking the latest task's success flag.

The script also validated the contract file itself. It had to be a readable regular file, not a symlink, owned by root and not writable by group or others. Its JSON schema rejected unexpected fields, duplicate or malformed group filters and incorrect policy values. It normalized filter order before comparison so an ordering-only difference would not cause an outage.

## Where the watcher stopped

The important ordering was:

```text
authoritative inventory
        |
        +--> generated sync contract --> installed contract
        |
        +--> expected job membership

scheduled service
        |
        +--> read live sync job
        +--> validate installed contract
        +--> compare normalized policy ------ mismatch --> exit failure
        +--> inspect capacity and task history
        +--> derive health and publish transitions
        +--> persist semantic health state
```

The live sync configuration had expanded to include additional guests. The repository inventory and renderer still described the older corpus. The contract gate rejected that divergence before the filesystem-capacity query and backup-task evaluation. That rejection was intentional: accepting a successful sync of the wrong corpus would provide misleading assurance about recovery coverage.

The initial audit reproduced the mismatch through the watcher's existing dry-run mode. Historical service records showed repeated failures. The timer remained active, while the health-state file retained its previous successful assessment. A consumer looking only at that state could see an old green result while the detector was no longer producing new evaluations.

An active timer establishes scheduling state. It does not establish completion of the service it activates. [The systemd timer specification](https://raw.githubusercontent.com/systemd/systemd/v258/man/systemd.timer.xml) describes that activation relationship; service outcome and the freshness of application evidence still need their own inspection.

## Why passing generation checks missed it

The renderer's check mode passed during the initial review. That result was correct within its boundary: checked-in generated files matched the checked-in inventory. Both shared the same omission.

A static consistency check answers whether two repository representations agree. A live reconciler answers whether declared job ownership and membership agree with the effective deployment. Snapshot inspection answers whether data actually exists. These are different comparisons, and the audit needed all three.

The live backup reconciler reported unexpected guest membership across job sources. Direct snapshot metadata then showed that the extra groups had encrypted destination copies. This prevented a tempting but incorrect remediation: dropping the extra filters just to make the checker green. Declared support scope then became explicit inventory policy.

## The recorded repair

The correction updated the authoritative inventory and regenerated its consumers together. The corrected declarations distinguished active coverage, deliberate exemptions and historical archive membership.

Generated watcher contracts were installed with hashes matching their repository outputs. The deployment record checked root ownership and protected file modes. Live backup-contract reconciliation and the watcher dry-run passed. On September 26, a subsequent ordinary scheduled watcher run completed with `overall=ok`, fresh sync evidence and policy match.

Contract reconciliation did not require launching a new sync, verification, prune or garbage-collection job. As a reusable review principle, changing membership should not silently change retention or source-deletion semantics; those are separate policy decisions.

| Recorded evidence | Conclusion supported |
| --- | --- |
| Generated files matched inventory before repair | Repository representations were internally consistent |
| Live membership exceeded declarations | Effective coverage and declared recovery scope had drifted |
| Extra groups had encrypted destination snapshots | The mismatch did not demonstrate missing copies |
| Installed contract hashes matched regenerated files | Intended policy reached the watcher consumers |
| Live reconciler and watcher dry-run passed | Recorded deployment matched the repaired contract |
| Natural scheduled watcher exited successfully | The ordinary detection path resumed |

## Keep checker health separate from backup health

The script's persistent state is designed for semantic transitions. Identical assessments suppress duplicate notifications; successful state writes use a temporary protected file followed by rename. This is useful alert behavior, but it is not an execution heartbeat. An unchanged healthy run can exit without replacing the semantic state, so that file's modification time alone is also an unreliable last-evaluation timestamp.

A useful follow-up design records two dimensions: the last completed assessment and the health of the assessor. For example, a consumer can retain `last_result=ok` while displaying `assessment=stale` after the service misses its expected completion window. Configuration mismatch should produce `assessment=failed`, not a new capacity or sync verdict. A separate success timestamp should advance on every completed evaluation, including unchanged results.

That redesign is a proposed improvement, not part of the recorded repair. It preserves the strict contract gate while preventing an old successful result from masquerading as current evidence. The practical review method is to inspect early exits, every state-write location and the distinction between scheduling, execution and domain results before trusting a green backup display.
