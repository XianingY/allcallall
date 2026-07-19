# Sandbox Supervisor Protocol

The sandbox supervisor is a static process bridge injected into an untrusted OCI
container. It does not implement MCP. The trusted Python Runner keeps ownership
of the MCP client session and communicates with the supervisor over a Unix
domain socket mounted from a memory-backed volume.

## Transport

Each frame has a five-byte header followed by the payload:

```text
+------+------------------------+------------------+
| kind | payload length (uint32) | payload bytes    |
| 1 B  | 4 B, big endian        | declared length  |
+------+------------------------+------------------+
```

The maximum payload is 1 MiB. Both peers must read the complete header and
payload before processing a frame. Truncated, oversized, unknown, or
out-of-order frames terminate the session.

| Kind | Name | Direction | Payload |
| --- | --- | --- | --- |
| `0x01` | `START` | Runner to supervisor | UTF-8 JSON launch request |
| `0x02` | `READY` | Supervisor to Runner | Empty |
| `0x03` | `STDIN` | Runner to supervisor | One UTF-8 JSON-RPC object, without newline |
| `0x04` | `STDOUT` | Supervisor to Runner | One UTF-8 JSON-RPC object, without newline |
| `0x05` | `ERROR` | Supervisor to Runner | UTF-8 JSON with a stable code and generic message |
| `0x06` | `EXIT` | Supervisor to Runner | UTF-8 JSON with `exit_code` |
| `0x07` | `CLOSE_STDIN` | Runner to supervisor | Empty |
| `0x08` | `CANCEL` | Runner to supervisor | Empty |

`START` is the first frame and has this shape:

```json
{
  "version": 1,
  "command": "/usr/local/bin/mcp-server",
  "args": ["--stdio"],
  "env": {"API_TOKEN": "in-memory-secret-value"},
  "timeout_ms": 30000
}
```

The supervisor launches the command directly, without a shell. After a
successful launch it sends `READY`. MCP stdio remains newline-delimited JSON;
the supervisor removes and restores the newline at the frame boundary.

## Security Invariants

- The socket accepts one Runner connection and is created with mode `0600`.
- Command, arguments, environment names and values, frame size, cumulative I/O,
  stderr, and execution time all have hard limits.
- Invalid UTF-8, non-JSON stdout, or output without a newline before the frame
  limit fails closed. Raw stdout and stderr are never logged or returned.
- Disconnect, timeout, `CANCEL`, or process exit closes stdin and terminates the
  complete child process group with bounded TERM/KILL escalation.
- `CLOSE_STDIN` is a best-effort graceful EOF. The Runner does not wait for
  `EXIT` during teardown; closing the socket transfers cleanup ownership to the
  supervisor, which still terminates and reaps the process group within bounds.
- Secret values exist only in the in-memory `START` frame and the child process
  environment. They must never enter a Pod spec, log, trace, receipt, or error
  payload.
- The Python Runner uses the official MCP SDK for initialize, pagination, and
  tool calls. The supervisor is only a process and byte-stream boundary.

## Rollout Boundary

Adding this protocol does not enable OCI execution in the shared Runner. OCI is
enabled only when the Kubernetes executor creates a per-execution gVisor Job
whose tool container runs the pinned user image and the injected supervisor.
Local shared Runner deployments must keep `SANDBOX_ALLOW_STDIO=0`.
