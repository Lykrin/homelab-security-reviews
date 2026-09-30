# SSH adapters: a local argument list does not remove the remote shell

**The privileged boundary in a VPN adapter can survive the web handler and reappear inside a remote command string.** Reviewing only `subprocess.run()` misses that distinction. This review follows inputs through a Python VPN controller, SSH, a container wrapper and its shell, then separates useful validation from assumptions about trusted configuration.

## Count parsers, not API calls

The transport invokes SSH through a Python argument list without requesting a local shell. Its last argument is a remote command string. That command can contain a container-execution wrapper followed by `sh -c` and a quoted inner command. The application therefore crosses several interpretations:

```text
Python argument list → SSH client
                     → remote login shell
                     → container wrapper arguments
                     → inner sh -c command
                     → file utility arguments
```

Python's subprocess documentation explains that it does not implicitly start a shell. That property protects local process argument separation. OpenSSH separately documents that noninteractive commands execute through the remote user's shell with its `-c` option. The explicit inner shell adds another parser. Neither API removes the other's interpretation rules. [Python subprocess security considerations](https://docs.python.org/3/library/subprocess.html#security-considerations), [OpenSSH command execution](https://man.openbsd.org/sshd.8#AUTHENTICATION).

The reviewed transport has an outer quoting function that encloses the entire inner command and escapes embedded single quotes. This protects the inner command as an argument to `sh -c`. But the inner command is still deliberately interpreted as shell syntax. In particular, a file-read helper constructs `cat` followed by the configured path without individually quoting that path. The write helper builds redirection to a configured temporary path and later constructs a move command the same way.

Outer quoting prevents an argument from prematurely escaping one layer. It does not make the contents of that argument inert at the next layer.

## Establish who controls each value

This is not evidence that an unauthenticated web request can inject a command. The privileged shell fragments identified here come from administrator-owned backend configuration: host alias, container identifiers, file paths and an explicit apply command. The inspected loader constructs backend objects from YAML; it does not validate a restricted command grammar for these fields.

If an attacker can edit that configuration, the explicit apply-command field already gives them substantial execution authority. A shell-injection finding must not pretend that compromising such a configuration is equivalent to controlling a low-privilege peer label. Nevertheless, unquoted paths are a concrete robustness and boundary problem: a legitimate path containing spaces can become several arguments, and shell metacharacters become syntax at the inner layer. Configuration is code-like authority here and should be handled accordingly.

| Value | Source | Where it goes | Review conclusion |
|---|---|---|---|
| SSH alias | Backend configuration | Local SSH argument list | Administrator-controlled destination; not a peer label |
| Container selector | Backend configuration | Remote wrapper command | Needs token validation or argument quoting |
| Configuration/table paths | Backend configuration | Inner shell and redirection | Outer command quoting does not quote these operands |
| Apply command | Backend configuration | Remote shell execution | Explicit code authority; document and restrict its owner |
| Peer display name | CLI/web input | JSON bookkeeping and local output filename | Validate before adapter call; not inserted into the reviewed shell command |
| Revoke name/address | CLI/web input | Registry equality lookup | Selected stored public key reaches configuration editing |
| File contents | Generated configuration | Encoded bytes on standard input | Transport data is separated from command construction |

The file writer base64-encodes content and sends it over standard input, rather than embedding a configuration body in shell source. That is a useful design choice: a quote or newline in file contents does not directly change the command text. Base64 is an encoding, however, not a confidentiality mechanism or proof that the output configuration is valid.

## Validation has its own edge

The common add command validates peer labels before constructing an adapter. It allows a bounded set of letters, digits, spaces and punctuation and rejects a double-dot substring. The web route strips surrounding whitespace before calling it. The adapter can also be invoked directly from Python, outside this facade; the inspected adapter method does not independently apply the label validator.

There is a specific CLI validation edge: the pattern uses `match()` with a final `$`. Python defines `$` as also matching immediately before a final newline. A label ending in one newline can therefore satisfy the pattern in the direct CLI path, even though newline is absent from the allowed character class. This is a source-and-language-semantics deduction, not an executed provisioning experiment. It matters because the original label is used later in local output filenames and bookkeeping. It is not a demonstrated remote command injection. [Python anchor semantics](https://docs.python.org/3/library/re.html), [Python full-string matching](https://docs.python.org/3/library/re.html#re.fullmatch).

Use full-string validation for a full-string contract, and decide deliberately whether whitespace should be accepted, normalized or rejected. Normalization only in the web layer leaves the CLI with a different contract.

```python
# Original generalized proposal; not an applied patch.
label_pattern = re.compile(r"[A-Za-z0-9][A-Za-z0-9 _.-]{0,63}")

def validate_label(label):
    if label_pattern.fullmatch(label) is None or ".." in label:
        raise ValueError("invalid label")
    return label
```

This narrow change addresses the anchor mismatch. It does not secure arbitrary filesystem destinations, existing symlinks, backend names or container arguments. Those are separate inputs with separate owners and constraints.

## A precise command-construction proposal

Where a shell remains necessary, build arguments for each layer rather than quote a preassembled mixture of syntax and data. This original illustrative pattern shows a read operation inside one container wrapper. It omits the controller's additional host-container layer and is not a drop-in implementation.

```python
# POSIX-shell model only; target paths and containers are server-owned.
inner_command = shlex.join(["cat", "--", selected_path])
remote_command = shlex.join([
    "docker", "exec", selected_container,
    "sh", "-c", inner_command,
])
subprocess.run(
    ["ssh", configured_alias, remote_command],
    check=True, capture_output=True,
)
```

The path is quoted for the shell that will parse `cat`'s arguments. The inner command is then quoted again as one argument for the shell parsing the wrapper. The end-of-options marker belongs to the file utility; it does not replace quoting. Redirection needs a quoted destination or a fixed helper that opens the file itself. Arbitrary apply commands require an explicit trusted-code policy, not an escaping function that claims to sanitize whole scripts.

OWASP recommends avoiding direct OS command execution where possible and combining parameterization with input validation when it remains necessary. A small fixed remote helper with structured input can make this contract easier to audit, provided it performs operations directly and does not interpolate those fields into another shell string. [OWASP command-injection defenses](https://cheatsheetseries.owasp.org/cheatsheets/OS_Command_Injection_Defense_Cheat_Sheet.html).

## Isolated adapter replay

On **30 September 2026**, an isolated archive of the original July transport was imported unchanged for a focused replay. A fake SSH executable evaluated the adapter's actual final remote-command string through two local shell layers. The fixture used synthetic files in a disposable directory, with no network connection, real SSH session, container execution or privileged operation.

The space-containing configured path split into separate shell arguments and failed. A configured path containing a semicolon caused the inner shell to execute an additional harmless action that created only the fixture's marker file. This dynamically supports the distinction between outer command quoting and inner operand quoting. It does not establish that a web user can set the affected configuration fields.

| Case | Result and evidence | Limit |
|---|---|---|
| Administrator-configured path containing spaces | Actual archived adapter command split operands and failed in the local replay | Synthetic files and fake SSH only |
| Administrator-configured path containing a semicolon | Actual archived adapter command created the harmless synthetic marker | Configuration authority was supplied by the fixture; no lower-authority writer established |
| CLI label ending with a newline | Source deduction from the pattern and documented Python anchor semantics | No runtime label/provisioning check reported here |
| Apostrophe or leading-dash path; proposed quoting and `fullmatch` changes | Further validation required | These cases and proposed fixes were not tested by this replay |

## A focused patch-validation plan

Existing tests inspect the configuration-write guard, pure peer-block manipulation and adapter delegation; a CLI test rejects a traversal-style label. Those assertions do not establish complete quoting across every parser or test the terminal-newline case. The new replay specifically exercises the configured path boundary; it does not validate every field, operation or adapter.

For a later patch, extend the disposable fixture to assert exact argument/value preservation for the space path and harmless marker case after operand quoting, then cover apostrophes and leading dashes. A label with a final newline should reject before adapter construction after the proposed validator change. Assert the value received and absence of unintended fixture side effects, not only a successful exit code. Neither proposed fix was applied or replayed.

No real configurations, keys, peers or remote hosts were accessed. The result is a confirmed administrator-configuration robustness boundary with two concrete patch directions: quote operands at their actual parser boundary and tighten full-string label validation. A lower-authority configuration writer would need separate evidence before assigning a privilege-escalation impact. No unauthenticated injection, real-host exploit or completed remediation is claimed.
