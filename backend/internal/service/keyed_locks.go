package service

import (
	"strconv"
	"sync"
)

// lockEntry 单个资源键的互斥锁与当前引用数。
type lockEntry struct {
	mu    *sync.Mutex
	count int // 已取锁或等待取锁的 goroutine 数
}

// keyedLocks 按字符串键提供互斥锁，不同键（不同项目/申请/凭证）之间互不阻塞。
// 用于在单进程内串行化同一关键资源的临界区；跨实例并发由数据库行锁与唯一索引保证。
//
// 条目采用引用计数：lock 时在持锁映射前自增计数，unlock 后归零即从映射删除，
// 因此长时间运行不会为历史资源残留锁记录；自增发生在 map 互斥区，
// 保证计数到 0 被删除时不会有 goroutine 正持有旧锁指针，避免新旧锁并存而失效。
type keyedLocks struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
}

func newKeyedLocks() *keyedLocks {
	return &keyedLocks{entries: make(map[string]*lockEntry)}
}

// lock 取得指定键的互斥锁，返回的释放函数幂等且仅可调用一次。
func (k *keyedLocks) lock(key string) func() {
	k.mu.Lock()
	e := k.entries[key]
	if e == nil {
		e = &lockEntry{mu: &sync.Mutex{}}
		k.entries[key] = e
	}
	e.count++ // 在 map 互斥区内计数，确保删除判定不会漏掉即将取锁者
	k.mu.Unlock()

	e.mu.Lock()

	var once sync.Once
	return func() {
		once.Do(func() {
			e.mu.Unlock()
			k.mu.Lock()
			e.count--
			if e.count <= 0 {
				delete(k.entries, key) // 空闲后立即回收，不残留
			}
			k.mu.Unlock()
		})
	}
}

// size 返回当前存活的锁条目数，仅用于测试/诊断。
func (k *keyedLocks) size() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.entries)
}

func projectKey(projectID uint) string { return "project:" + uitoa(projectID) }
func applicationKey(id uint) string    { return "application:" + uitoa(id) }
func voucherKey(id uint) string        { return "voucher:" + uitoa(id) }

func uitoa(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
