const DEFAULT_STATUSES = ['idle', 'enroute', 'charging', 'offline']
const DEFAULT_REGIONS = ['north', 'south', 'east', 'west']

const seededTrucks = Array.from({ length: 50 }).map((_, index) => {
  const region = DEFAULT_REGIONS[index % DEFAULT_REGIONS.length]
  const status = DEFAULT_STATUSES[index % DEFAULT_STATUSES.length]
  const latitude = 37.75 + (Math.random() - 0.5) * 0.5
  const longitude = -122.45 + (Math.random() - 0.5) * 0.5
  return {
    id: `mock-${index}`,
    name: `Truck ${index + 1}`,
    latitude,
    longitude,
    status,
    region,
    speed: Math.round(Math.random() * 60),
  }
})

export async function fetchTrucks(signal) {
  try {
    const response = await fetch('/api/trucks', { signal })
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`)
    }
    const payload = await response.json()
    return Array.isArray(payload) ? payload : seededTrucks
  } catch (error) {
    console.warn('Falling back to seeded trucks', error)
    return seededTrucks
  }
}

export function createTruckSubscriber({ onMessage, onError }) {
  let retryCount = 0
  let socket
  let closed = false

  const start = () => {
    if (closed) return
    const protocol = location.protocol === 'https:' ? 'wss' : 'ws'
    socket = new WebSocket(`${protocol}://${location.host}/ws/trucks`)

    socket.onmessage = (event) => {
      retryCount = 0
      try {
        const update = JSON.parse(event.data)
        onMessage?.(update)
      } catch (err) {
        onError?.(err)
      }
    }

    socket.onerror = (event) => {
      onError?.(event)
    }

    socket.onclose = () => {
      if (closed) return
      retryCount += 1
      const delay = Math.min(30000, 1000 * 2 ** retryCount)
      setTimeout(start, delay)
    }
  }

  start()

  return () => {
    closed = true
    if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) {
      socket.close()
    }
  }
}

export { DEFAULT_REGIONS, DEFAULT_STATUSES }
