// Package acp is a minimal Agent Client Protocol (v1) client: JSON-RPC 2.0
// over an agent's stdio, the method subset webmux uses, and the session
// update kinds it renders. It depends only on the standard library, never
// starts processes and carries no webmux semantics; the caller owns the
// agent process and hands its pipes to NewConn.
//
// Types follow the pinned schema in testdata (schema-v1.27.0 schema.json
// and meta.json); TestSchemaGuard fails when a method or field name used
// here is missing from it. To upgrade, replace both files from the
// agentclientprotocol/agent-client-protocol release tag and rerun the tests.
//
// Content safety: nothing here logs message content, prompts, tool input or
// output, permission options, form fields, agent stderr or rejected lines.
// Logs carry method names, error codes, protocol error kinds and lengths;
// RPCError.Error omits the agent's message text.
//
// testdata/fakeagent is a scriptable fake agent for tests and E2E, built
// with
//
//	go build -o <dir>/fakeagent ./internal/acp/testdata/fakeagent
//
// See its package comment for modes, flags and fault injection.
package acp
