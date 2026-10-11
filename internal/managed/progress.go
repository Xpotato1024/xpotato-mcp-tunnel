package managed

import (
	"context"
	"fmt"
	"io"
	"time"
)

const progressInterval = 5 * time.Second
const fileOperationTimeout = 10 * time.Minute

// Reports only our fixed stage labels, never child output, URLs or environment.
// Join the heartbeat before returning so it cannot cross an input prompt.
func runStage(report func(string), label string, interval time.Duration, work func() error) error {
	if report == nil {
		if err := work(); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		return nil
	}
	started := time.Now()
	report(label + " — 開始")
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				report(fmt.Sprintf("%s — 処理中（経過 %s）", label, time.Since(started).Truncate(time.Second)))
			}
		}
	}()
	err := work()
	close(stop)
	<-done
	state := "完了"
	if err != nil {
		state = "失敗"
	}
	report(fmt.Sprintf("%s — %s（経過 %s）", label, state, time.Since(started).Truncate(time.Second)))
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}
