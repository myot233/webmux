package agenttrust

// kimiTrustWorkspaceEnv trusts only the current process's workspace. Kimi
// documents this as equivalent to accepting its interactive folder-trust
// prompt, without writing a persistent trust record.
const kimiTrustWorkspaceEnv = "KIMI_CODE_TRUST_WORKSPACE"

// trustKimi adds Kimi's per-process workspace trust unless the session or
// server environment already made an explicit choice, including a falsy one.
func (t *Trust) trustKimi(_ string, env map[string]string) error {
	if _, ok := env[kimiTrustWorkspaceEnv]; ok {
		return nil
	}
	if _, ok := t.lookupEnv(kimiTrustWorkspaceEnv); ok {
		return nil
	}
	env[kimiTrustWorkspaceEnv] = "1"
	return nil
}
