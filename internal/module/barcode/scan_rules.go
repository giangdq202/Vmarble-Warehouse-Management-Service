package barcode

import "github.com/vmarble/warehouse-management-service/internal/domain"

func isCheckpointAllowed(last *ScanCheckpoint, next ScanCheckpoint) bool {
	if last == nil {
		return next == CheckpointCNCComplete
	}
	return checkpointOrder(next) == checkpointOrder(*last)+1
}

func expectedWOStatusForCheckpoint(c ScanCheckpoint) (domain.WorkOrderStatus, bool) {
	switch c {
	case CheckpointCNCComplete:
		return domain.WOInCutting, true
	case CheckpointFinishedGoods:
		return domain.WOInProcessing, true
	case CheckpointShipped:
		return domain.WOCompleted, true
	default:
		return "", false
	}
}
