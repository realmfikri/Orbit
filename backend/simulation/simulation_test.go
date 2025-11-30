package simulation

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestGreatCircleDistanceAndBearing(t *testing.T) {
	start := Point{Lat: 0, Lon: 0}
	end := Point{Lat: 0, Lon: 90}
	distance := GreatCircleDistance(start, end)
	bearing := InitialBearing(start, end)

	if math.Abs(distance-10007543) > 500 {
		t.Fatalf("unexpected distance: got %.0f", distance)
	}
	if math.Abs(bearing-90) > 0.5 {
		t.Fatalf("unexpected bearing: got %.2f", bearing)
	}
}

func TestStepTowardsConverges(t *testing.T) {
	start := Point{Lat: 47.0, Lon: -122.0}
	end := Point{Lat: 47.0, Lon: -122.001}
	speed := 30.0
	dt := 1.0

	current := start
	for i := 0; i < 20; i++ {
		var reached bool
		current, reached = StepTowards(current, end, speed, dt)
		if reached {
			break
		}
	}

	if GreatCircleDistance(current, end) > 1 {
		t.Fatalf("expected to converge to target; remaining distance: %.2f", GreatCircleDistance(current, end))
	}
}

func TestLifecycleStartStop(t *testing.T) {
	cfg := Config{
		NumTrucks:      5,
		Seed:           99,
		SpeedMin:       5,
		SpeedMax:       5,
		UpdateInterval: 20 * time.Millisecond,
		StartPoints:    []Point{{Lat: 0, Lon: 0}},
		EndPoints:      []Point{{Lat: 1, Lon: 1}},
	}

	manager := NewManager(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("failed to start simulation: %v", err)
	}

	time.Sleep(3 * cfg.UpdateInterval)
	manager.Stop()

	snapshot := manager.Trucks()
	time.Sleep(2 * cfg.UpdateInterval)
	snapshotAfter := manager.Trucks()

	for i := range snapshot {
		if snapshot[i].Lat != snapshotAfter[i].Lat || snapshot[i].Lon != snapshotAfter[i].Lon {
			t.Fatalf("expected trucks to stop updating after Stop")
		}
	}
}

func TestUpdateFrequency(t *testing.T) {
	cfg := Config{
		NumTrucks:      1,
		Seed:           123,
		SpeedMin:       1,
		SpeedMax:       1,
		UpdateInterval: 30 * time.Millisecond,
		StartPoints:    []Point{{Lat: 0, Lon: 0}},
		EndPoints:      []Point{{Lat: 10, Lon: 0}},
	}

	manager := NewManager(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer manager.Stop()

	initial := manager.Trucks()[0]
	expectedDistance := cfg.UpdateInterval.Seconds() * initial.Speed * 3

	time.Sleep(3*cfg.UpdateInterval + 10*time.Millisecond)
	updated := manager.Trucks()[0]

	distanceTraveled := GreatCircleDistance(Point{Lat: initial.Lat, Lon: initial.Lon}, Point{Lat: updated.Lat, Lon: updated.Lon})
	if distanceTraveled < expectedDistance*0.8 {
		t.Fatalf("truck did not advance at expected frequency: moved %.4fm want at least %.4fm", distanceTraveled, expectedDistance*0.8)
	}
}

func TestDeterministicSeedingAndStateMutation(t *testing.T) {
	cfg := Config{
		NumTrucks:      3,
		Seed:           7,
		SpeedMin:       2,
		SpeedMax:       2,
		UpdateInterval: 200 * time.Millisecond,
		StartPoints: []Point{
			{Lat: 10, Lon: 10},
			{Lat: 20, Lon: 20},
		},
		EndPoints: []Point{{Lat: 15, Lon: 15}},
	}

	manager1 := NewManager(cfg)
	manager2 := NewManager(cfg)

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	if err := manager1.Start(ctx1); err != nil {
		t.Fatalf("start manager1: %v", err)
	}
	if err := manager2.Start(ctx2); err != nil {
		t.Fatalf("start manager2: %v", err)
	}
	defer manager1.Stop()
	defer manager2.Stop()

	snap1 := manager1.Trucks()
	snap2 := manager2.Trucks()

	for i := range snap1 {
		if snap1[i].Lat != snap2[i].Lat || snap1[i].Lon != snap2[i].Lon || snap1[i].Speed != snap2[i].Speed {
			t.Fatalf("deterministic seeding failed at index %d", i)
		}
	}

	time.Sleep(cfg.UpdateInterval + 20*time.Millisecond)
	afterUpdate := manager1.Trucks()

	moved := false
	for i := range snap1 {
		if afterUpdate[i].Lat != snap1[i].Lat || afterUpdate[i].Lon != snap1[i].Lon {
			moved = true
		}
	}
	if !moved {
		t.Fatalf("expected trucks to mutate state after a tick")
	}
}

func TestRouteLoopsAndAdvancesWaypoints(t *testing.T) {
	cfg := Config{
		NumTrucks:         1,
		Seed:              5,
		SpeedMin:          200,
		SpeedMax:          200,
		WaypointsPerRoute: 2,
		LoopRoutes:        true,
		UpdateInterval:    50 * time.Millisecond,
		StartPoints:       []Point{{Lat: 0, Lon: 0}},
		EndPoints:         []Point{{Lat: 0, Lon: 0.001}},
	}

	manager := NewManager(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	defer manager.Stop()

	deadline := time.Now().Add(4 * time.Second)
	for {
		truck := manager.Trucks()[0]
		distanceFromStart := GreatCircleDistance(Point{Lat: 0, Lon: 0}, Point{Lat: truck.Lat, Lon: truck.Lon})
		if distanceFromStart < 30 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected truck to loop back near start, distance %.2f", distanceFromStart)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
