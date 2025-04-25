// Copyright 2025 Fengzhi Li
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cache

import (
	"runtime"
	"sync"
	"time"
)

type item struct {
	object any
	expire int64
}

type cache struct {
	items  map[string]item
	expire time.Duration
	mu     sync.RWMutex
	stop   chan bool
}

// This trick ensures that the eviction goroutine (which--granted it
// was enabled--is running DeleteExpired on c forever) does not keep
// the returned C object from being garbage collected. When it is
// garbage collected, the finalizer stops the eviction goroutine, after
// which c can be collected.
// Please refer to https://github.com/patrickmn/go-cache
type Cache struct {
	*cache
}

func New(expireDuration time.Duration, evictionInterval time.Duration) *Cache {
	if expireDuration <= 0 || evictionInterval <= 0 {
		return nil
	}
	items := make(map[string]item)
	c := &cache{
		expire: expireDuration,
		items:  items,
		stop:   make(chan bool),
	}
	C := &Cache{c}
	runtime.SetFinalizer(C, stop)
	go Eviction(c, evictionInterval, c.stop)
	return C
}

func Eviction(c *cache, interval time.Duration, stop chan bool) {
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			now := time.Now().UnixNano()
			c.mu.Lock()
			for k, v := range c.items {
				if now > v.expire {
					delete(c.items, k)
				}
			}
			c.mu.Unlock()
		case <-stop:
			ticker.Stop()
			return
		}
	}
}

func stop(c *Cache) {
	c.stop <- true
}

func (c *cache) Set(k string, x any) {
	c.mu.Lock()
	c.items[k] = item{
		object: x,
		expire: time.Now().Add(c.expire).UnixNano(),
	}
	c.mu.Unlock()
}

func (c *cache) Get(key string) (value any, valid bool) {
	c.mu.RLock()
	item, found := c.items[key]
	if found {
		if time.Now().UnixNano() <= item.expire {
			c.mu.RUnlock()
			return item.object, true
		}
	}
	c.mu.RUnlock()
	return nil, false
}
