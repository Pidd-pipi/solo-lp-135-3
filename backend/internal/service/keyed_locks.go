package service

import (
	"strconv"
	"sync"
)

// keyedLocks 按字符串键提供互斥锁，不同键（不同项目/申请）之间互不阻塞。
// 用于在单进程内串行化同一关键资源的额度变更临界区；
// 跨实例的并发由数据库行锁（FOR UPDATE）与唯一索引保证。
type keyedLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newKeyedLocks() *keyedLocks {
	return &keyedLocks{locks: make(map[string]*sync.Mutex)}
}

func (k *keyedLocks) get(key string) *sync.Mutex {
	k.mu.Lock()
	defer k.mu.Unlock()
	m, ok := k.locks[key]
	if !ok {
		m = &sync.Mutex{}
		k.locks[key] = m
	}
	return m
}

func (k *keyedLocks) lock(key string) func() {
	m := k.get(key)
	m.Lock()
	return m.Unlock
}

func projectKey(projectID uint) string { return "project:" + uitoa(projectID) }
func applicationKey(id uint) string    { return "application:" + uitoa(id) }
func voucherKey(id uint) string        { return "voucher:" + uitoa(id) }

func uitoa(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
