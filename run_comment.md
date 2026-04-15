<details><summary>Click to view code changes: run.go</summary>

```go
func StartService(cfg *config.Config, configPath string, localPassword string) {
	if cfg.AuthDir != "" {
		dbPath := filepath.Join(filepath.Dir(cfg.AuthDir), "traffic_logs.db")
		if err := trafficlog.InitGlobalLogger(dbPath); err != nil {
			log.Warnf("Failed to initialize traffic logger DB: %v", err)
		} else {
			trafficlog.StartWriting()
		}
	}
// ...
	if logger := trafficlog.GetGlobalLogger(); logger != nil {
		logger.Drain()
		logger.Close()
	}
}

func StartServiceBackground(cfg *config.Config, configPath string, localPassword string) (cancel func(), done <-chan struct{}) {
	if cfg.AuthDir != "" {
		dbPath := filepath.Join(filepath.Dir(cfg.AuthDir), "traffic_logs.db")
		if err := trafficlog.InitGlobalLogger(dbPath); err != nil {
			log.Warnf("Failed to initialize traffic logger DB: %v", err)
		} else {
			trafficlog.StartWriting()
		}
	}
// ...
	go func() {
		defer close(doneCh)
		defer func() {
			if logger := trafficlog.GetGlobalLogger(); logger != nil {
				logger.Drain()
				logger.Close()
			}
		}()
```
</details>

**What**: Wired `TrafficLogger`'s SQLite init and teardown routines into the application root execution (`StartService` and `StartServiceBackground`).
**Why**: Ensures logs are automatically captured and safely flushed to disk during startup/shutdown to avoid database corruption or WAL logging loss.
**Who**: Any CLI Proxy API instances booting up will leverage this to instantiate `traffic_logs.db` adjacent to their configured `.cli-proxy-api` dir.
**How**: Bound `trafficlog.InitGlobalLogger` during runtime boot with fully evaluated config directories, and tied `Drain()` and `Close()` inside `defer` functions aligned to component cleanup.
