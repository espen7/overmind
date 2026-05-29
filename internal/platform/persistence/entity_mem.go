package persistence

import (
	"context"
	"reflect"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type EntityStore[K comparable, E any] interface {
	UpsertMany(ctx context.Context, entities map[K]E) error
	DeleteMany(ctx context.Context, keys []K) error
}

// EntityMem 提供实体级脏追踪。
// 第一版先对齐 antares-main 的“按实体追踪增量”语义，不默认做字段级 patch。
type EntityMem[K comparable, E any] struct {
	store    EntityStore[K, E]
	entities func() map[K]E

	snapshots    map[K]E
	dirtyUpserts map[K]E
	dirtyDeletes map[K]struct{}

	maxRetries int
	backoff    time.Duration
}

func NewEntityMem[K comparable, E any](store EntityStore[K, E], entities func() map[K]E) *EntityMem[K, E] {
	return &EntityMem[K, E]{
		store:        store,
		entities:     entities,
		snapshots:    make(map[K]E),
		dirtyUpserts: make(map[K]E),
		dirtyDeletes: make(map[K]struct{}),
		maxRetries:   3,
		backoff:      200 * time.Millisecond,
	}
}

func (m *EntityMem[K, E]) MarkClean() error {
	current := m.entities()
	m.snapshots = make(map[K]E, len(current))
	m.dirtyUpserts = make(map[K]E)
	m.dirtyDeletes = make(map[K]struct{})

	for key, entity := range current {
		clone, err := cloneEntity(entity)
		if err != nil {
			return err
		}
		m.snapshots[key] = clone
	}

	return nil
}

func (m *EntityMem[K, E]) TraceEntities() error {
	current := m.entities()

	for key := range m.snapshots {
		if _, ok := current[key]; !ok {
			m.dirtyDeletes[key] = struct{}{}
			delete(m.dirtyUpserts, key)
		}
	}

	for key, entity := range current {
		clone, err := cloneEntity(entity)
		if err != nil {
			return err
		}

		snapshot, ok := m.snapshots[key]
		if !ok || !reflect.DeepEqual(snapshot, clone) {
			m.dirtyUpserts[key] = clone
			delete(m.dirtyDeletes, key)
		}
	}

	return nil
}

func (m *EntityMem[K, E]) Flush(ctx context.Context) error {
	upserts := make(map[K]E, len(m.dirtyUpserts))
	for key, entity := range m.dirtyUpserts {
		upserts[key] = entity
	}

	deletes := make([]K, 0, len(m.dirtyDeletes))
	for key := range m.dirtyDeletes {
		deletes = append(deletes, key)
	}

	if len(upserts) == 0 && len(deletes) == 0 {
		return nil
	}

	if len(upserts) > 0 {
		if err := retry(ctx, m.maxRetries, m.backoff, func(runCtx context.Context) error {
			return m.store.UpsertMany(runCtx, upserts)
		}); err != nil {
			return err
		}
	}

	if len(deletes) > 0 {
		if err := retry(ctx, m.maxRetries, m.backoff, func(runCtx context.Context) error {
			return m.store.DeleteMany(runCtx, deletes)
		}); err != nil {
			return err
		}
	}

	for key, entity := range upserts {
		m.snapshots[key] = entity
		delete(m.dirtyUpserts, key)
	}

	for _, key := range deletes {
		delete(m.snapshots, key)
		delete(m.dirtyDeletes, key)
	}

	return nil
}

func retry(ctx context.Context, maxRetries int, backoff time.Duration, fn func(context.Context) error) error {
	var lastErr error
	delay := backoff

	for attempt := 0; attempt < maxRetries; attempt++ {
		if err := fn(ctx); err != nil {
			lastErr = err
			if attempt == maxRetries-1 {
				break
			}

			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			delay *= 2
			continue
		}
		return nil
	}

	return lastErr
}

func cloneEntity[E any](entity E) (E, error) {
	payload, err := bson.Marshal(entity)
	if err != nil {
		var zero E
		return zero, err
	}

	var clone E
	if err := bson.Unmarshal(payload, &clone); err != nil {
		var zero E
		return zero, err
	}
	return clone, nil
}
