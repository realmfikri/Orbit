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

// ConfigUpdate captures partial updates that can be applied to a running simulation.
type ConfigUpdate struct {
	NumTrucks      *int
	UpdateInterval *time.Duration
	BoundingBox    *BoundingBox
}

func normalizeConfig(cfg Config) Config {
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

	return cfg
}

func cloneConfig(cfg Config) Config {
	cfg.StartPoints = append([]Point{}, cfg.StartPoints...)
	cfg.EndPoints = append([]Point{}, cfg.EndPoints...)
	cfg.RouteBounds = append([]BoundingBox{}, cfg.RouteBounds...)
	return cfg
}

// Manager coordinates simulated truck updates using a shared ticker.
type Manager struct {
	mu     sync.RWMutex
	trucks map[string]*Truck
	routes map[string]*routeState

	cfg      Config
	initial  Config
	rand     *rand.Rand
	ticker   *time.Ticker
	lastTick time.Time

	ctx      context.Context
	cancel   context.CancelFunc
	baseCtx  context.Context
	wg       sync.WaitGroup
	tickSubs []chan time.Time

	started bool
}

// NewManager creates a manager with deterministic seeding and defaults.
func NewManager(cfg Config) *Manager {
	cfg = normalizeConfig(cfg)

	return &Manager{
		trucks:  make(map[string]*Truck, cfg.NumTrucks),
		routes:  make(map[string]*routeState, cfg.NumTrucks),
		cfg:     cfg,
		initial: cfg,
		rand:    rand.New(rand.NewSource(cfg.Seed)),
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
	if m.baseCtx == nil {
		m.baseCtx = ctx
	}
	m.ctx, m.cancel = context.WithCancel(m.baseCtx)
	m.ticker = time.NewTicker(m.cfg.UpdateInterval)
	m.lastTick = time.Now()
	m.tickSubs = make([]chan time.Time, 0, m.cfg.NumTrucks)

	for i := 0; i < m.cfg.NumTrucks; i++ {
		truck := m.buildTruck(i)
		m.trucks[truck.ID] = truck
	}

	for _, truck := range m.trucks {
		tickCh := make(chan time.Time, 1)
		m.tickSubs = append(m.tickSubs, tickCh)
		m.wg.Add(1)
		go m.runTruck(truck, tickCh)
	}

	m.wg.Add(1)
	go m.runTicker()

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

// Config returns a copy of the current simulation configuration.
func (m *Manager) Config() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneConfig(m.cfg)
}

// InitialConfig returns the initial configuration the manager was created with.
func (m *Manager) InitialConfig() Config {
	return cloneConfig(m.initial)
}

// ApplyConfig restarts the simulation using the provided configuration.
func (m *Manager) ApplyConfig(cfg Config) error {
	m.mu.RLock()
	baseCtx := m.baseCtx
	started := m.started
	m.mu.RUnlock()

	if !started {
		return fmt.Errorf("simulation not started")
	}
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	m.Stop()

	cfg = cloneConfig(normalizeConfig(cfg))
	m.mu.Lock()
	m.resetLocked(cfg)
	m.mu.Unlock()

	return m.Start(baseCtx)
}

// ApplyUpdate merges the provided updates into the current configuration and restarts the simulation.
func (m *Manager) ApplyUpdate(update ConfigUpdate) (Config, error) {
	cfg := m.Config()

	if update.NumTrucks != nil {
		cfg.NumTrucks = *update.NumTrucks
	}
	if update.UpdateInterval != nil {
		cfg.UpdateInterval = *update.UpdateInterval
	}
	if update.BoundingBox != nil {
		cfg.RouteBounds = []BoundingBox{*update.BoundingBox}
	}

	if err := m.ApplyConfig(cfg); err != nil {
		return Config{}, err
	}
	return m.Config(), nil
}

func (m *Manager) resetLocked(cfg Config) {
	m.cfg = cfg
	m.trucks = make(map[string]*Truck, cfg.NumTrucks)
	m.routes = make(map[string]*routeState, cfg.NumTrucks)
	m.rand = rand.New(rand.NewSource(cfg.Seed))
	m.tickSubs = nil
	m.ticker = nil
	m.lastTick = time.Time{}
}

// Started returns whether the simulation is currently running.
func (m *Manager) Started() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
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

func (m *Manager) runTruck(truck *Truck, tickCh <-chan time.Time) {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-tickCh:
			start := time.Now()
			m.advanceTruck(truck)
			updateDuration.Observe(time.Since(start).Seconds())
		}
	}
}

func (m *Manager) runTicker() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		case t := <-m.ticker.C:
			m.recordTickLatency(t)
			for _, ch := range m.tickSubs {
				select {
				case ch <- t:
				default:
				}
			}
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

func (m *Manager) recordTickLatency(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.lastTick.IsZero() {
		m.lastTick = now
		return
	}

	delta := now.Sub(m.lastTick)
	m.lastTick = now
	tickLatency.Observe(delta.Seconds())
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
