package aoi

import (
	"fmt"
	"sort"
	"sync"
)

type Point struct {
	X int32
	Y int32
}

type Grid struct {
	cellSize int32
	mu       sync.RWMutex
	points   map[int64]Point
	cells    map[string]map[int64]struct{}
}

func NewGrid(width int32, height int32, cellSize int32) *Grid {
	_ = width
	_ = height
	return &Grid{
		cellSize: cellSize,
		points:   make(map[int64]Point),
		cells:    make(map[string]map[int64]struct{}),
	}
}

// Upsert moves an entity between cells without forcing callers to delete first.
func (g *Grid) Upsert(id int64, x int32, y int32) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if existing, ok := g.points[id]; ok {
		delete(g.cells[g.cellKey(existing.X, existing.Y)], id)
	}

	point := Point{X: x, Y: y}
	g.points[id] = point

	key := g.cellKey(x, y)
	if _, ok := g.cells[key]; !ok {
		g.cells[key] = make(map[int64]struct{})
	}
	g.cells[key][id] = struct{}{}
}

// VisibleTo returns every entity inside the caller's 3x3 neighboring cell area.
func (g *Grid) VisibleTo(id int64) []int64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	point, ok := g.points[id]
	if !ok {
		return nil
	}

	cx, cy := g.cell(point.X), g.cell(point.Y)
	visible := make([]int64, 0)
	for dx := int32(-1); dx <= 1; dx++ {
		for dy := int32(-1); dy <= 1; dy++ {
			key := fmt.Sprintf("%d:%d", cx+dx, cy+dy)
			for other := range g.cells[key] {
				if other == id {
					continue
				}
				visible = append(visible, other)
			}
		}
	}
	sort.Slice(visible, func(i int, j int) bool { return visible[i] < visible[j] })
	return visible
}

func (g *Grid) cell(value int32) int32 {
	return value / g.cellSize
}

func (g *Grid) cellKey(x int32, y int32) string {
	return fmt.Sprintf("%d:%d", g.cell(x), g.cell(y))
}
