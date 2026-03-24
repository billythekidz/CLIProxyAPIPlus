package contextmemory

import "sync/atomic"

type ReductionMetricsSnapshot struct {
	CaptureSuccessTotal uint64
	CaptureErrorTotal   uint64
	AppliedTotal        uint64
	SkippedTotal        uint64
	ErrorTotal          uint64
	TokensBeforeTotal   uint64
	TokensAfterTotal    uint64
}

var reductionMetrics struct {
	captureOK    atomic.Uint64
	captureError atomic.Uint64
	applied      atomic.Uint64
	skipped      atomic.Uint64
	errors       atomic.Uint64
	tokensBefore atomic.Uint64
	tokensAfter  atomic.Uint64
}

func RecordCaptureSuccess() {
	reductionMetrics.captureOK.Add(1)
}

func RecordCaptureError() {
	reductionMetrics.captureError.Add(1)
}

func RecordReductionApplied(tokensBefore, tokensAfter int) {
	reductionMetrics.applied.Add(1)
	if tokensBefore > 0 {
		reductionMetrics.tokensBefore.Add(uint64(tokensBefore))
	}
	if tokensAfter > 0 {
		reductionMetrics.tokensAfter.Add(uint64(tokensAfter))
	}
}

func RecordReductionSkipped() {
	reductionMetrics.skipped.Add(1)
}

func RecordReductionError() {
	reductionMetrics.errors.Add(1)
}

func SnapshotReductionMetrics() ReductionMetricsSnapshot {
	return ReductionMetricsSnapshot{
		CaptureSuccessTotal: reductionMetrics.captureOK.Load(),
		CaptureErrorTotal:   reductionMetrics.captureError.Load(),
		AppliedTotal:        reductionMetrics.applied.Load(),
		SkippedTotal:        reductionMetrics.skipped.Load(),
		ErrorTotal:          reductionMetrics.errors.Load(),
		TokensBeforeTotal:   reductionMetrics.tokensBefore.Load(),
		TokensAfterTotal:    reductionMetrics.tokensAfter.Load(),
	}
}
