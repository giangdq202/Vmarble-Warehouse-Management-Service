package barcode

import "github.com/vmarble/warehouse-management-service/internal/domain"

func isCheckpointAllowed(last *ScanCheckpoint, next ScanCheckpoint) bool {
	if last == nil {
		return next == CheckpointCNCComplete
	}
	// Non-linear QC branching: QC_FAILED allows a retry (another QC scan),
	// QC_PASSED unlocks FINISHED_GOODS. All other transitions are sequential.
	switch *last {
	case CheckpointCNCComplete:
		return next == CheckpointQCPassed || next == CheckpointQCFailed
	case CheckpointQCFailed:
		// Operator may re-scan after rework; allow another QC attempt.
		return next == CheckpointQCPassed || next == CheckpointQCFailed
	case CheckpointQCPassed:
		return next == CheckpointFinishedGoods
	case CheckpointFinishedGoods:
		return next == CheckpointShipped
	default:
		return false
	}
}

func expectedWOStatusForCheckpoint(c ScanCheckpoint) (domain.WorkOrderStatus, bool) {
	switch c {
	case CheckpointCNCComplete:
		return domain.WOInCutting, true
	case CheckpointQCPassed, CheckpointQCFailed:
		return domain.WOInProcessing, true
	case CheckpointFinishedGoods:
		return domain.WOInProcessing, true
	case CheckpointShipped:
		return domain.WOCompleted, true
	default:
		return "", false
	}
}
