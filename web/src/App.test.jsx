import { render, screen, waitFor } from '@testing-library/react'
import { vi, beforeEach, afterEach, describe, it, expect } from 'vitest'
import App from './App'

const sampleTrucks = [
  { id: 't1', name: 'Truck One', latitude: 37.77, longitude: -122.43, status: 'idle', region: 'north', speed: 45 },
  { id: 't2', name: 'Truck Two', latitude: 37.78, longitude: -122.44, status: 'enroute', region: 'south', speed: 52 },
]

class MockWebSocket {
  static OPEN = 1
  static CONNECTING = 0
  static CLOSING = 2
  static CLOSED = 3

  constructor() {
    this.readyState = MockWebSocket.OPEN
    setTimeout(() => this.onopen?.(), 0)
  }

  close() {
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.()
  }

  send() {}
}

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => sampleTrucks }))
  vi.stubGlobal('WebSocket', MockWebSocket)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('App', () => {
  it('loads the map', async () => {
    render(<App />)
    const map = await screen.findByTestId('fleet-map')
    expect(map).toBeInTheDocument()
    await waitFor(() => expect(screen.getByTestId('truck-count')).toHaveTextContent(`${sampleTrucks.length} vehicles`))
  })

  it('renders truck markers using canvas renderer', async () => {
    render(<App />)
    await waitFor(() => expect(screen.getByTestId('truck-count')).toHaveTextContent(`${sampleTrucks.length} vehicles`))
    await waitFor(() => expect(screen.getByTestId('cluster-count').textContent).not.toBe('0'))
  })
})
