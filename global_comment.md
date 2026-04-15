<details><summary>Click to view code changes: global.go & writer.go</summary>

```go
// ... global.go ...
// InitGlobalLogger initializes the DB, Logger, and Pruner and stores them.
func InitGlobalLogger(dbPath string) error {
	db, err := NewStore(dbPath)
	if err != nil {
		return err
	}
	logger := NewTrafficLogger(db)
	SetGlobalLogger(logger)
	globalPruner = NewPruner(db, dbPath)
	return nil
}

// StartWriting starts the internal writing and pruning routines.
func StartWriting() {
	if globalPruner != nil {
		globalPruner.Start()
	}
}

// Close fully stops and cleans up global traffic logging routines.
func Close() {
...

// ... writer.go ...
func (l *trafficLoggerImpl) Drain() {
	// Signal writer task to finish processing everything in the channel and exit
	select {
	case <-l.done:
		// already closed
	default:
		close(l.done)
	}
	l.wg.Wait()
}
```
</details>

**What**: Exported `InitGlobalLogger`, `StartWriting` and `Close` wrapper functions, and fixed data-race panic inside `Drain()`.
**Why**: Ensures smooth encapsulation so the app root isn't heavily exposed to `trafficlog` internals like the `DB` reference, while `Drain()` multiple invocations are prevented from panicking down the stream.
**Who**: Proxy servers managing logging state.
**How**: Centralized references via global state variables; guarded close channels with a non-blocking `select` wrapper.
