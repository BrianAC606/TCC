package redis_lock

import (
	"TCC/third_party"
	"context"
	"sync"
	"testing"
	"time"
)

func Test_block_lock(t *testing.T) {
	addr := "127.0.0.1:6379"
	password := ""

	client := third_party.NewClient("tcp", addr, password)

	lock1 := NewRedisLock("test1", client, WithExpireSeconds(2))
	lock2 := NewRedisLock("test1", client, WithBlock(), WithBlockWaitingSeconds(1))

	wg := sync.WaitGroup{}
	ctx := context.Background()

	wg.Add(1)

	t.Log("lock1 work")
	if err := lock1.Lock(ctx); err != nil {
		t.Error(err)
		return
	}
	wg.Done()

	time.Sleep(1000 * time.Millisecond)

	wg.Add(1)
	t.Log("lock2 work")
	if err := lock2.Lock(ctx); err != nil {
		t.Error(err)
		return
	}
	wg.Done()

	wg.Wait()
	err := lock1.Unlock(ctx)
	if err != nil {
		t.Error(err)
	}
	err = lock2.Unlock(ctx)
	if err != nil {
		t.Error(err)
	}
	t.Log("success")
}

func Test_non_block_lock(t *testing.T) {
	addr := "127.0.0.1:6379"
	password := ""

	client := third_party.NewClient("tcp", addr, password)

	lock1 := NewRedisLock("test2", client, WithExpireSeconds(1))
	lock2 := NewRedisLock("test2", client, WithExpireSeconds(1))

	wg := sync.WaitGroup{}
	ctx := context.Background()

	wg.Add(1)

	go func() {
		defer wg.Done()
		t.Log("lock1 work")
		if err := lock1.Lock(ctx); err != nil {
			t.Error(err)
			return
		}
	}()

	wg.Add(1)

	t.Log("lock2 work")
	if err := lock2.Lock(ctx); err != nil {
		if IsRetryableErr(err) {
			t.Error("lock acquired by other: ", err)
			return
		}
		t.Error(err)
		return
	}

	wg.Done()

	wg.Wait()
	_ = lock1.Unlock(ctx)
	_ = lock2.Unlock(ctx)
	t.Log("success")
}
