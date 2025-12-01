import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { vi, beforeEach, afterEach, describe, it, expect } from 'vitest'
import App from './App'

const sampleTrucks = [
  { id: 't1', name: 'Truck One', latitude: 37.77, longitude: -122.43, status: 'idle', region: 'north', speed: 45 },
  { id: 't2', name: 'Truck Two', latitude: 37.78, longitude: -122.44, status: 'enroute', region: 'south', speed: 52 },
]

const sampleConfig = { numTrucks: 50, updateIntervalMs: 1000, boundingBox: null }

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

let fetchMock
let postResponse
let lastPostBody

beforeEach(() => {
  postResponse = { ...sampleConfig }
  lastPostBody = null
  fetchMock = vi.fn((url, options = {}) => {
    if (url === '/api/trucks') {
      return Promise.resolve({ ok: true, json: async () => sampleTrucks })
    }
    if (url === '/api/simulation/config') {
      if (options.method === 'POST') {
        lastPostBody = options.body
        return Promise.resolve({ ok: true, json: async () => postResponse, text: async () => '' })
      }
      return Promise.resolve({ ok: true, json: async () => sampleConfig })
    }
    return Promise.reject(new Error(`Unhandled request for ${url}`))
  })

  vi.stubGlobal('fetch', fetchMock)
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

  it('updates simulation config via sidebar controls', async () => {
    render(<App />)

    const truckInput = await screen.findByTestId('num-trucks-input')
    const intervalInput = screen.getByTestId('update-interval-input')

    expect(truckInput).toHaveValue(sampleConfig.numTrucks)

    postResponse = { numTrucks: 75, updateIntervalMs: 750, boundingBox: null }

    fireEvent.change(truckInput, { target: { value: '75' } })
    fireEvent.change(intervalInput, { target: { value: '750' } })
    fireEvent.click(screen.getByTestId('apply-config'))

    await screen.findByText(/configuration saved/i)

    const payload = JSON.parse(lastPostBody)
    expect(payload.numTrucks).toBe(75)
    expect(payload.updateIntervalMs).toBe(750)

    await waitFor(() => expect(truckInput).toHaveValue(postResponse.numTrucks))
  })
})
