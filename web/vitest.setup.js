import '@testing-library/jest-dom'

const noop = () => {}
const mockContext = new Proxy(
  {
    canvas: {},
    clearRect: noop,
    fillRect: noop,
    drawImage: noop,
    getImageData: () => ({ data: [] }),
    putImageData: noop,
    createImageData: () => [],
    setTransform: noop,
    translate: noop,
    beginPath: noop,
    arc: noop,
    fill: noop,
    stroke: noop,
    save: noop,
    restore: noop,
    scale: noop,
    moveTo: noop,
    lineTo: noop,
    closePath: noop,
    rect: noop,
    clip: noop,
  },
  {
    get(target, prop) {
      if (prop in target) return target[prop]
      return noop
    },
  },
)

HTMLCanvasElement.prototype.getContext = () => mockContext

if (!window.requestAnimationFrame) {
  window.requestAnimationFrame = (cb) => setTimeout(cb, 0)
}
if (!window.cancelAnimationFrame) {
  window.cancelAnimationFrame = (id) => clearTimeout(id)
}
