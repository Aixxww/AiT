package datafetch

import (
	"context"
	"sync/atomic"
)

// TA-5 (docs/tier-a-improvement-plan-20261120.md): per-fetch observability.
//
// FetchWithOptions attaches a fetchStats to its context; doPublic records
// every request that failed after its retries. The counters are per fetch
// call, so concurrent fetches on one DataFetcher never mix. Request
// behaviour (retries, rate limiting, timeouts) is unchanged.

type fetchStatsKey struct{}

type fetchStats struct {
	failedRequests int64
}

func withFetchStats(ctx context.Context, st *fetchStats) context.Context {
	return context.WithValue(ctx, fetchStatsKey{}, st)
}

func fetchStatsFrom(ctx context.Context) *fetchStats {
	if ctx == nil {
		return nil
	}
	st, _ := ctx.Value(fetchStatsKey{}).(*fetchStats)
	return st
}

// recordFailure counts one request that failed after its retries. Nil-safe.
func (st *fetchStats) recordFailure(path string, err error) {
	if st == nil {
		return
	}
	atomic.AddInt64(&st.failedRequests, 1)
}

func (st *fetchStats) failed() int {
	if st == nil {
		return 0
	}
	return int(atomic.LoadInt64(&st.failedRequests))
}
