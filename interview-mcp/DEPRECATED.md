# DEPRECATED — moved to the standalone `allcallall-agent-runtime` repository

This directory is a **stale mirror** of the interview MCP server code that now
lives in the standalone repository
[`allcallall-agent-runtime`](https://github.com/XianingY/allcallall-agent-runtime).

**Do not edit files here.** The authoritative source is
`allcallall-agent-runtime/services/interview-mcp` (or the equivalent package in
the standalone repo). Edits to this in-tree copy do not affect the deployed
runtime, which builds from the standalone repository.

The local interview stack (`scripts/interview-stack.sh`) references the
`interview-mcp` docker-compose **service name**, whose image is built from the
standalone repo. See `contracts/README.md` for cross-repository contract
governance.
