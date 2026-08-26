package port

// Observer is process-local alarm instrumentation. All methods must be
// nil-safe (a nil Observer or a nil concrete value is a no-op).
//
// Open-alarm counts are incremented on a brand-new ticket and decremented on
// Close. SOE merge and ack do not change the gauge. SetOpenCount is for
// startup calibration only — never a per-scrape SQL COUNT.
type Observer interface {
	AlarmOpened(source string)
	AlarmClosed(source string)
	SetOpenCount(source string, n int)
	AckConflict()
	CloseConflict()
}
