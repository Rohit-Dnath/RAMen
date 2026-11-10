package store

import "github.com/Rohit-Dnath/RAMen/internal/vector"

// VSet stores a vector under id within the collection at key. expireUnix is
// the Unix second the item expires at, 0 for no expiry.
func (s *Store) VSet(key, id string, vec []float32, meta string, expireUnix int64) error {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	var c *vector.Collection
	if found {
		var ok bool
		c, ok = e.val.(*vector.Collection)
		if !ok {
			return ErrWrongType
		}
	} else {
		c = vector.NewCollection()
		sh.m[key] = &entry{val: c}
	}
	if err := c.Set(id, vec, meta, expireUnix); err != nil {
		return err
	}
	// Inserting counts as an access, so a brand-new item is never the LRU pick.
	c.Touch(id, s.now().UnixNano())
	return nil
}

// VTouch marks a vector as just-used for LRU purposes. A missing key or id is
// a no-op.
func (s *Store) VTouch(key, id string) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return
	}
	if c, ok := e.val.(*vector.Collection); ok {
		c.Touch(id, s.now().UnixNano())
	}
}

// VEnforceCap bounds the collection at key to max items. Expired items are
// swept first, since reclaiming a dead entry is always better than evicting a
// live one; only then does approximated LRU evict live items. max <= 0 means
// unbounded and does nothing.
func (s *Store) VEnforceCap(key string, max int) {
	if max <= 0 {
		return
	}
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return
	}
	c, ok := e.val.(*vector.Collection)
	if !ok {
		return
	}
	if c.Len() > max {
		c.SweepExpired(s.now().Unix())
	}
	c.EvictLRU(max)
}

// VSearch returns the top-k nearest vectors in the collection at key.
func (s *Store) VSearch(key string, query []float32, k int) ([]vector.Result, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return nil, nil
	}
	c, ok := e.val.(*vector.Collection)
	if !ok {
		return nil, ErrWrongType
	}
	return c.Search(query, k, s.now().Unix())
}

// VDel removes a vector id from the collection at key.
func (s *Store) VDel(key, id string) (bool, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return false, nil
	}
	c, ok := e.val.(*vector.Collection)
	if !ok {
		return false, ErrWrongType
	}
	removed := c.Del(id)
	if c.Len() == 0 {
		delete(sh.m, key)
	}
	return removed, nil
}

// VCard returns the number of vectors in the collection at key.
func (s *Store) VCard(key string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return 0, nil
	}
	c, ok := e.val.(*vector.Collection)
	if !ok {
		return 0, ErrWrongType
	}
	return c.Len(), nil
}

// VDim returns the dimension of the collection at key (0 if empty/missing).
func (s *Store) VDim(key string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return 0, nil
	}
	c, ok := e.val.(*vector.Collection)
	if !ok {
		return 0, ErrWrongType
	}
	return c.Dim, nil
}
