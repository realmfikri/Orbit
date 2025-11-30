package simulation

import (
	"math"
	"math/rand"
)

const earthRadiusMeters = 6371000.0

func degreesToRadians(deg float64) float64 {
	return deg * math.Pi / 180
}

func radiansToDegrees(rad float64) float64 {
	return rad * 180 / math.Pi
}

// GreatCircleDistance returns the distance in meters between two coordinates.
func GreatCircleDistance(start, end Point) float64 {
	lat1 := degreesToRadians(start.Lat)
	lat2 := degreesToRadians(end.Lat)
	lon1 := degreesToRadians(start.Lon)
	lon2 := degreesToRadians(end.Lon)

	dLat := lat2 - lat1
	dLon := lon2 - lon1
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusMeters * c
}

// InitialBearing returns the compass bearing in degrees from start to end.
func InitialBearing(start, end Point) float64 {
	lat1 := degreesToRadians(start.Lat)
	lat2 := degreesToRadians(end.Lat)
	dLon := degreesToRadians(end.Lon - start.Lon)

	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	bearing := math.Atan2(y, x)
	bearingDeg := radiansToDegrees(bearing)

	if bearingDeg < 0 {
		bearingDeg += 360
	}
	return bearingDeg
}

// StepTowards advances from start toward end given speed (m/s) and seconds elapsed.
// It returns the new coordinate and a boolean indicating whether the target was reached.
func StepTowards(start, end Point, speed float64, seconds float64) (Point, bool) {
	distance := GreatCircleDistance(start, end)
	if distance == 0 {
		return end, true
	}

	step := speed * seconds
	if step >= distance {
		return end, true
	}

	bearing := degreesToRadians(InitialBearing(start, end))
	angularDistance := step / earthRadiusMeters

	lat1 := degreesToRadians(start.Lat)
	lon1 := degreesToRadians(start.Lon)

	lat2 := math.Asin(math.Sin(lat1)*math.Cos(angularDistance) + math.Cos(lat1)*math.Sin(angularDistance)*math.Cos(bearing))
	lon2 := lon1 + math.Atan2(math.Sin(bearing)*math.Sin(angularDistance)*math.Cos(lat1), math.Cos(angularDistance)-math.Sin(lat1)*math.Sin(lat2))

	return Point{Lat: radiansToDegrees(lat2), Lon: radiansToDegrees(lon2)}, false
}

// BoundingBox defines a rectangular geographic area.
type BoundingBox struct {
	MinLat float64
	MaxLat float64
	MinLon float64
	MaxLon float64
}

// BoundingBoxFromPoints returns the min/max extents that contain the provided points.
func BoundingBoxFromPoints(points []Point) BoundingBox {
	if len(points) == 0 {
		return BoundingBox{}
	}
	minLat, maxLat := points[0].Lat, points[0].Lat
	minLon, maxLon := points[0].Lon, points[0].Lon
	for _, p := range points[1:] {
		if p.Lat < minLat {
			minLat = p.Lat
		}
		if p.Lat > maxLat {
			maxLat = p.Lat
		}
		if p.Lon < minLon {
			minLon = p.Lon
		}
		if p.Lon > maxLon {
			maxLon = p.Lon
		}
	}
	return BoundingBox{MinLat: minLat, MaxLat: maxLat, MinLon: minLon, MaxLon: maxLon}
}

// RandomRouteWithinBounds returns count random points within the bounding box.
func RandomRouteWithinBounds(rng *rand.Rand, bounds BoundingBox, count int) []Point {
	if count <= 0 {
		return nil
	}

	latSpan := bounds.MaxLat - bounds.MinLat
	lonSpan := bounds.MaxLon - bounds.MinLon
	if latSpan == 0 {
		latSpan = 1
	}
	if lonSpan == 0 {
		lonSpan = 1
	}

	points := make([]Point, 0, count)
	for i := 0; i < count; i++ {
		lat := bounds.MinLat + rng.Float64()*latSpan
		lon := bounds.MinLon + rng.Float64()*lonSpan
		points = append(points, Point{Lat: lat, Lon: lon})
	}
	return points
}
