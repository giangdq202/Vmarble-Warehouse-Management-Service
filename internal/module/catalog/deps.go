package catalog

import (
	"context"

	"github.com/google/uuid"
)

// PolicyAuditLogger persists a min-remnant policy threshold change to the
// shared audit ledger (BR-K08). Implementations are wired in cmd/server/main.go;
// the catalog module does not import inventory directly.
//
// Errors are best-effort logged by the service: a transient audit-write
// failure must never roll back the threshold mutation itself, since the
// mutation is the source of truth and an admin can re-issue the PATCH if the
// audit row is missing.
type PolicyAuditLogger interface {
	LogMinRemnantPolicyChange(ctx context.Context, in MinRemnantPolicyChange) error
}

// MinRemnantPolicyChange carries the before/after threshold values plus the
// actor identity. Used by the catalog service to emit an audit row when an
// admin updates a material's min_remnant policy.
type MinRemnantPolicyChange struct {
	MaterialID   uuid.UUID
	ActorID      uuid.UUID
	PrevLengthMM int
	PrevWidthMM  int
	NewLengthMM  int
	NewWidthMM   int
}
