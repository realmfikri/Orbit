import { useEffect, useMemo, useState } from 'react'
import {
  CircleMarker,
  MapContainer,
  TileLayer,
  Tooltip,
  useMap,
} from 'react-leaflet'
import L from 'leaflet'
import Supercluster from 'supercluster'
import {
  createTruckSubscriber,
  DEFAULT_REGIONS,
  DEFAULT_STATUSES,
  fetchTrucks,
} from './api/truckService'
import './App.css'
import 'leaflet/dist/leaflet.css'

const canvasRenderer = L.canvas({ padding: 0.5 })

const statusColors = {
  idle: '#2dd4bf',
  enroute: '#60a5fa',
  charging: '#fbbf24',
  offline: '#9ca3af',
}

function SimulationControls({ isRunning, speedMultiplier, onToggle, onSpeedChange }) {
  return (
    <div className="panel">
      <div className="panel-header">Simulation</div>
      <div className="control-row">
        <button type="button" onClick={onToggle}>
          {isRunning ? 'Stop' : 'Start'}
        </button>
        <label className="control-label" htmlFor="speed">Speed</label>
        <select
          id="speed"
          value={speedMultiplier}
          onChange={(event) => onSpeedChange(Number(event.target.value))}
        >
          {[0.5, 1, 2, 4].map((value) => (
            <option key={value} value={value}>{`${value}x`}</option>
          ))}
        </select>
      </div>
    </div>
  )
}

function FilterControls({ filters, onChange, availableRegions }) {
  return (
    <div className="panel">
      <div className="panel-header">Filters</div>
      <div className="control-row">
        <label className="control-label" htmlFor="status-filter">Status</label>
        <select
          id="status-filter"
          value={filters.status}
          onChange={(event) => onChange({ ...filters, status: event.target.value })}
        >
          <option value="all">All</option>
          {DEFAULT_STATUSES.map((status) => (
            <option key={status} value={status}>{status}</option>
          ))}
        </select>
      </div>
      <div className="control-row">
        <label className="control-label" htmlFor="region-filter">Region</label>
        <select
          id="region-filter"
          value={filters.region}
          onChange={(event) => onChange({ ...filters, region: event.target.value })}
        >
          <option value="all">All</option>
          {availableRegions.map((region) => (
            <option key={region} value={region}>{region}</option>
          ))}
        </select>
      </div>
    </div>
  )
}

function ClusteredTrucks({ trucks }) {
  const map = useMap()
  const [bounds, setBounds] = useState(null)
  const [zoom, setZoom] = useState(map.getZoom())
  const [clusterReady, setClusterReady] = useState(false)

  const clusterer = useMemo(
    () => new Supercluster({ radius: 80, maxZoom: 19 }),
    [],
  )

  const features = useMemo(
    () => trucks.map((truck) => ({
      type: 'Feature',
      geometry: {
        type: 'Point',
        coordinates: [truck.longitude, truck.latitude],
      },
      properties: truck,
    })),
    [trucks],
  )

  useEffect(() => {
    setClusterReady(false)
    clusterer.load(features)
    setClusterReady(true)
  }, [clusterer, features])

  useEffect(() => {
    const handleMove = () => {
      setBounds(map.getBounds())
      setZoom(map.getZoom())
    }

    map.on('moveend', handleMove)
    map.whenReady(handleMove)

    return () => {
      map.off('moveend', handleMove)
    }
  }, [map])

  const clusters = useMemo(() => {
    if (!features.length || !clusterReady) return []
    const bbox = bounds
      ? [bounds.getWest(), bounds.getSouth(), bounds.getEast(), bounds.getNorth()]
      : [-180, -90, 180, 90]
    const computed = clusterer.getClusters(bbox, Math.round(zoom))
    if (computed.length === 0) {
      return features.map((feature) => ({
        ...feature,
        properties: { ...feature.properties, cluster: false },
      }))
    }
    return computed
  }, [bounds, clusterReady, clusterer, features, zoom])

  return (
    <>
      <span className="sr-only" data-testid="cluster-count">{clusters.length}</span>
      {clusters.map((cluster) => {
        const [longitude, latitude] = cluster.geometry.coordinates
        const isCluster = cluster.properties.cluster

        if (isCluster) {
          const count = cluster.properties.point_count
          return (
            <CircleMarker
              key={`cluster-${cluster.id}`}
          center={[latitude, longitude]}
          radius={16}
          renderer={canvasRenderer}
          pathOptions={{ color: '#1f2937', fillColor: '#111827', fillOpacity: 0.8 }}
        >
          <Tooltip direction="top" offset={[0, -4]} permanent className="cluster-tooltip">
            {count} trucks
          </Tooltip>
        </CircleMarker>
          )
        }

    const truck = cluster.properties
    const color = statusColors[truck.status] || '#f97316'

    return (
      <CircleMarker
        key={truck.id}
        center={[latitude, longitude]}
        radius={7}
        renderer={canvasRenderer}
        pathOptions={{ color, fillColor: color, fillOpacity: 0.9 }}
      >
        <Tooltip direction="top" offset={[0, -4]} opacity={0.9}>
          <div className="tooltip">
            <div className="tooltip-title">{truck.name}</div>
            <div className="tooltip-row">Status: {truck.status}</div>
            <div className="tooltip-row">Region: {truck.region}</div>
            <div className="tooltip-row">Speed: {truck.speed} mph</div>
          </div>
        </Tooltip>
      </CircleMarker>
      )
      })}
    </>
  )
}

function applyFilters(trucks, filters) {
  return trucks.filter((truck) => {
    const matchesStatus = filters.status === 'all' || truck.status === filters.status
    const matchesRegion = filters.region === 'all' || truck.region === filters.region
    return matchesStatus && matchesRegion
  })
}

export default function App() {
  const [trucks, setTrucks] = useState([])
  const [isRunning, setIsRunning] = useState(true)
  const [speedMultiplier, setSpeedMultiplier] = useState(1)
  const [filters, setFilters] = useState({ status: 'all', region: 'all' })
  const [error, setError] = useState(null)

  useEffect(() => {
    const controller = new AbortController()
    fetchTrucks(controller.signal)
      .then(setTrucks)
      .catch((err) => setError(err.message))
    return () => controller.abort()
  }, [])

  useEffect(() => {
    if (!isRunning) return undefined

    const unsubscribe = createTruckSubscriber({
      onMessage: (update) => {
        setTrucks((current) => {
          if (!update || !update.id) return current
          const normalized = { ...update }
          if (update.lat != null && update.latitude == null) {
            normalized.latitude = update.lat
          }
          if (update.lng != null && update.longitude == null) {
            normalized.longitude = update.lng
          }
          return current.map((truck) => (truck.id === update.id ? { ...truck, ...normalized } : truck))
        })
      },
      onError: (event) => setError(event?.message ?? 'Live updates unavailable'),
    })

    return unsubscribe
  }, [isRunning])

  useEffect(() => {
    if (!isRunning) return undefined
    const interval = setInterval(() => {
      setTrucks((current) =>
        current.map((truck) => ({
          ...truck,
          latitude: truck.latitude + (Math.random() - 0.5) * 0.001 * speedMultiplier,
          longitude: truck.longitude + (Math.random() - 0.5) * 0.001 * speedMultiplier,
        })),
      )
    }, 5000 / speedMultiplier)

    return () => clearInterval(interval)
  }, [isRunning, speedMultiplier])

  const filteredTrucks = useMemo(
    () => applyFilters(trucks, filters),
    [filters, trucks],
  )

  const regions = useMemo(() => {
    const available = new Set(DEFAULT_REGIONS)
    trucks.forEach((truck) => available.add(truck.region))
    return Array.from(available)
  }, [trucks])

  return (
    <div className="app-shell">
      <header className="app-header">
        <div>
          <h1>Fleet map</h1>
          <p className="subtitle">OpenStreetMap base layer with canvas-rendered trucks</p>
        </div>
        <div className="stats">
          <span data-testid="truck-count">{filteredTrucks.length} vehicles</span>
          {error && <span className="error">{error}</span>}
        </div>
      </header>

      <main className="layout">
        <section className="map-panel">
          <div className="map-wrapper" data-testid="fleet-map">
            <MapContainer
              center={[37.77, -122.43]}
              zoom={11}
              minZoom={2}
              className="map"
              preferCanvas
              attributionControl
            >
              <TileLayer
                attribution='&copy; <a href="https://www.openstreetmap.org/">OpenStreetMap</a> contributors'
                url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
              />
              <ClusteredTrucks trucks={filteredTrucks} />
            </MapContainer>
          </div>
        </section>

        <aside className="sidebar">
          <SimulationControls
            isRunning={isRunning}
            speedMultiplier={speedMultiplier}
            onSpeedChange={setSpeedMultiplier}
            onToggle={() => setIsRunning((value) => !value)}
          />
          <FilterControls
            filters={filters}
            onChange={setFilters}
            availableRegions={regions}
          />
        </aside>
      </main>
    </div>
  )
}
