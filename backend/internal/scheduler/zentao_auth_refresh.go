package scheduler

import (
	"context"
	"log"
	"time"
	"zenmind/internal/config"
	"zenmind/internal/db"
	"zenmind/internal/zentaoauth"
)

// StartPeriodicZentaoAuthRefresh 定时使用已保存的禅道凭证重建 Redis 会话，
// 让用户在授权成功后无需频繁手动重复输入密码。
func StartPeriodicZentaoAuthRefresh(ctx context.Context) {
	mins := config.ClampZentaoAuthRefreshMinutes(config.Global.ZentaoAuthRefreshMinutes)
	if mins <= 0 {
		log.Printf("[scheduler] zentao auth auto-refresh disabled")
		return
	}
	go func() {
		for {
			timer := time.NewTimer(time.Duration(mins) * time.Minute)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return
			case <-timer.C:
				refreshSavedZentaoSessions(ctx)
			}
		}
	}()
}

func refreshSavedZentaoSessions(ctx context.Context) {
	refs, err := db.ListZentaoCredentialRefs()
	if err != nil {
		log.Printf("[scheduler] zentao auth refresh list credentials failed: %v", err)
		return
	}
	if len(refs) == 0 {
		return
	}

	okCount := 0
	failCount := 0
	for _, ref := range refs {
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, _, err := zentaoauth.RefreshSavedSession(runCtx, ref.Username)
		cancel()
		if err != nil {
			failCount++
			log.Printf("[scheduler] zentao auth refresh failed username=%s account=%s: %v", ref.Username, ref.ZentaoAccount, err)
			continue
		}
		okCount++
	}
	log.Printf("[scheduler] zentao auth refresh done total=%d ok=%d failed=%d", len(refs), okCount, failCount)
}
