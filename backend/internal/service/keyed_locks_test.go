package service

import (
	"sync"
	"sync/atomic"
	"testing"
)

// 释放后锁条目应立即回收，不残留。
func TestKeyedLocksReleasesIdleEntries(t *testing.T) {
	kl := newKeyedLocks()
	if kl.size() != 0 {
		t.Fatalf("fresh lock store should be empty, got %d", kl.size())
	}

	for i := 0; i < 100; i++ {
		unlock := kl.lock(voucherKey(uint(i)))
		unlock()
	}
	if got := kl.size(); got != 0 {
		t.Fatalf("idle lock entries must be reclaimed, got %d residual", got)
	}

	// 释放函数幂等：重复调用不应使计数变为负数或残留。
	unlock := kl.lock(applicationKey(7))
	unlock()
	unlock()
	if got := kl.size(); got != 0 {
		t.Fatalf("idempotent unlock must not leave/reintroduce entry, got %d", got)
	}
}

// 同一键并发临界区互斥，不同键互不阻塞；结束后全部回收。
func TestKeyedLocksSerializesSameKey(t *testing.T) {
	kl := newKeyedLocks()
	const n = 50

	var (
		wg        sync.WaitGroup
		inSection int32
		maxSeen   int32
		mu        sync.Mutex
	)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			unlock := kl.lock(projectKey(1))
			cur := atomic.AddInt32(&inSection, 1)
			mu.Lock()
			if cur > maxSeen {
				maxSeen = cur
			}
			mu.Unlock()
			atomic.AddInt32(&inSection, -1)
			unlock()
		}()
	}
	close(start)
	wg.Wait()

	if maxSeen != 1 {
		t.Fatalf("same-key critical section must be exclusive, observed max concurrency %d", maxSeen)
	}
	if got := kl.size(); got != 0 {
		t.Fatalf("all entries should be reclaimed after concurrent use, got %d", got)
	}
}

// 持锁期间条目存活，全部释放后回收（验证引用计数覆盖等待者）。
func TestKeyedLocksHoldsEntryWhileContended(t *testing.T) {
	kl := newKeyedLocks()
	key := voucherKey(99)
	unlock1 := kl.lock(key)
	done := make(chan struct{})
	go func() {
		unlock2 := kl.lock(key)
		unlock2()
		close(done)
	}()
	// 第二个 goroutine 已自增计数并在等待锁，此时必须保留唯一一条目。
	if got := kl.size(); got != 1 {
		unlock1()
		t.Fatalf("contended key should retain exactly one entry, got %d", got)
	}
	unlock1()
	<-done
	if got := kl.size(); got != 0 {
		t.Fatalf("entry should be reclaimed once all waiters finish, got %d", got)
	}
}
