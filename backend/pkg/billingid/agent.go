package billingid

// AgentOperation is the canonical ledger and operation reconciliation key.
func AgentOperation(id string) string { return "agent:" + id }
