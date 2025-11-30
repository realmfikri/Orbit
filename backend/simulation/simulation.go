package simulation

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// TruckStatus represents the current lifecycle state of a truck in the simulation.
type TruckStatus string

const (
	TruckStatusEnRoute TruckStatus = "enroute"
	TruckStatusIdle    TruckStatus = "idle"
)

// Truck describes the simulated vehicle state.
type Truck struct {
	ID           string
	Lat          float64
	Lon          float64
	Speed        float64
	CurrentRoute string
	Status       TruckStatus
}

// Point represents a coordinate used for routing.
type Point struct {
	Lat float64
	Lon float64
}

// Config drives the parameters of the simulation.
type Config struct {
	NumTrucks         int
	Seed              int64
	SpeedMin          float64
	SpeedMax          float64
	StartPoints       []Point
	EndPoints         []Point
	WaypointsPerRoute int
	RouteBounds       []BoundingBox
	LoopRoutes        bool
	UpdateInterval    time.Duration
}

const (
	defaultNumTrucks = 2000
	defaultSeed      = int64(42)
	defaultSpeedMin  = 10
	defaultSpeedMax  = 25
	defaultInterval  = time.Second
)

type routeState struct {
	waypoints []Point
	legIndex  int
	loop      bool
}

// Manager coordinates simulated truck updates using a shared ticker.
type Manager struct {
	mu     sync.RWMutex
	trucks map[string]*Truck
	routes map[string]*routeState

	cfg    Config
	rand   *rand.Rand
	ticker *time.Ticker

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	started bool
}

// NewManager creates a manager with deterministic seeding and defaults.
func NewManager(cfg Config) *Manager {
	if cfg.NumTrucks <= 0 {
		cfg.NumTrucks = defaultNumTrucks
	}
	if cfg.Seed == 0 {
		cfg.Seed = defaultSeed
	}
	if cfg.SpeedMin <= 0 {
		cfg.SpeedMin = defaultSpeedMin
	}
	if cfg.SpeedMax <= cfg.SpeedMin {
		cfg.SpeedMax = defaultSpeedMax
	}
	if cfg.WaypointsPerRoute < 2 {
		cfg.WaypointsPerRoute = 2
	}
	if len(cfg.StartPoints) == 0 {
		cfg.StartPoints = []Point{{Lat: 47.6062, Lon: -122.3321}}
	}
	if len(cfg.EndPoints) == 0 {
		cfg.EndPoints = []Point{{Lat: 37.7749, Lon: -122.4194}}
	}
	if cfg.UpdateInterval == 0 {
		cfg.UpdateInterval = defaultInterval
	}

	return &Manager{
		trucks: make(map[string]*Truck, cfg.NumTrucks),
		routes: make(map[string]*routeState, cfg.NumTrucks),
		cfg:    cfg,
		rand:   rand.New(rand.NewSource(cfg.Seed)),
	}
}

// Start spins up goroutines per truck and begins ticking.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return fmt.Errorf("simulation already started")
	}
	m.started = true
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.ticker = time.NewTicker(m.cfg.UpdateInterval)

	for i := 0; i < m.cfg.NumTrucks; i++ {
		truck := m.buildTruck(i)
		m.trucks[truck.ID] = truck
	}

	for _, truck := range m.trucks {
		m.wg.Add(1)
		go m.runTruck(truck)
	}

	return nil
}

// Stop cancels the simulation and waits for goroutines to finish.
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	cancel := m.cancel
	ticker := m.ticker
	m.started = false
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if ticker != nil {
		ticker.Stop()
	}
	m.wg.Wait()
}

// Trucks returns a snapshot copy of all simulated trucks.
func (m *Manager) Trucks() []Truck {
	m.mu.RLock()
	defer m.mu.RUnlock()
	trucks := make([]Truck, 0, len(m.trucks))
	for _, t := range m.trucks {
		copy := *t
		trucks = append(trucks, copy)
	}
	sort.Slice(trucks, func(i, j int) bool {
		return trucks[i].ID < trucks[j].ID
	})
	return trucks
}

func (m *Manager) runTruck(truck *Truck) {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.ticker.C:
			m.advanceTruck(truck)
		}
	}
}

func (m *Manager) advanceTruck(truck *Truck) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.routes[truck.ID]
	if state == nil {
		return
	}

	if len(state.waypoints) < 2 {
		truck.Status = TruckStatusIdle
		return
	}

	if state.legIndex >= len(state.waypoints) {
		state.legIndex = len(state.waypoints) - 1
	}

	target := state.waypoints[state.legIndex]
	current := Point{Lat: truck.Lat, Lon: truck.Lon}
	next, reached := StepTowards(current, target, truck.Speed, m.cfg.UpdateInterval.Seconds())

	truck.Lat = next.Lat
	truck.Lon = next.Lon
	truck.CurrentRoute = state.label()
	truck.Status = TruckStatusEnRoute

	if reached {
		state.advance(next, m.rand)
	}
}

func (m *Manager) buildTruck(index int) *Truck {
	start := m.pickStartpoint()
	end := m.pickEndpoint()
	waypoints := m.buildRoute(start, end)
	truck := &Truck{
		ID:           fmt.Sprintf("truck-%04d", index+1),
		Lat:          start.Lat,
		Lon:          start.Lon,
		Speed:        m.pickSpeed(),
		CurrentRoute: fmt.Sprintf("%s_to_%s", pointLabel(start), pointLabel(end)),
		Status:       TruckStatusEnRoute,
	}
	m.routes[truck.ID] = &routeState{
		waypoints: waypoints,
		legIndex:  1,
		loop:      m.cfg.LoopRoutes,
	}
	return truck
}

func (m *Manager) pickSpeed() float64 {
	delta := m.cfg.SpeedMax - m.cfg.SpeedMin
	return m.cfg.SpeedMin + m.rand.Float64()*delta
}

func (m *Manager) pickStartpoint() Point {
	return m.cfg.StartPoints[m.rand.Intn(len(m.cfg.StartPoints))]
}

func (m *Manager) pickEndpoint() Point {
	return m.cfg.EndPoints[m.rand.Intn(len(m.cfg.EndPoints))]
}

func pointLabel(p Point) string {
	return fmt.Sprintf("%.3f,%.3f", p.Lat, p.Lon)
}

func (m *Manager) buildRoute(start, end Point) []Point {
	waypoints := []Point{start}
	if m.cfg.WaypointsPerRoute > 2 {
		bounds := m.defaultBounds()
		if len(m.cfg.RouteBounds) > 0 {
			bounds = m.cfg.RouteBounds[m.rand.Intn(len(m.cfg.RouteBounds))]
		}
		intermediate := RandomRouteWithinBounds(m.rand, bounds, m.cfg.WaypointsPerRoute-2)
		waypoints = append(waypoints, intermediate...)
	}
	return append(waypoints, end)
}

func (m *Manager) defaultBounds() BoundingBox {
	allPoints := append([]Point{}, m.cfg.StartPoints...)
	allPoints = append(allPoints, m.cfg.EndPoints...)
	if len(allPoints) == 0 {
		return BoundingBox{MinLat: -90, MaxLat: 90, MinLon: -180, MaxLon: 180}
	}
	return BoundingBoxFromPoints(allPoints)
}

func (r *routeState) label() string {
	if len(r.waypoints) == 0 {
		return ""
	}
	if r.legIndex >= len(r.waypoints) {
		return pointLabel(r.waypoints[len(r.waypoints)-1])
	}
	return pointLabel(r.waypoints[r.legIndex])
}

func (r *routeState) advance(current Point, rng *rand.Rand) {
	if len(r.waypoints) == 0 {
		return
	}
	if r.legIndex < len(r.waypoints)-1 {
		r.legIndex++
		return
	}

	if r.loop {
		r.legIndex = 0
		return
	}

	if len(r.waypoints) == 1 {
		return
	}

	rest := make([]Point, len(r.waypoints)-1)
	copy(rest, r.waypoints[:len(r.waypoints)-1])
	rng.Shuffle(len(rest), func(i, j int) {
		rest[i], rest[j] = rest[j], rest[i]
	})
	r.waypoints = append([]Point{current}, rest...)
	if len(r.waypoints) > 1 {
		r.legIndex = 1
	} else {
		r.legIndex = 0
	}
}
