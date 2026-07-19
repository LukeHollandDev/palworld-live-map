import '@testing-library/jest-dom/vitest'

class ResizeObserverStub implements ResizeObserver {
  disconnect() {}
  observe() {}
  unobserve() {}
}

globalThis.ResizeObserver = ResizeObserverStub

HTMLDialogElement.prototype.showModal = function showModal() {
  this.open = true
}

HTMLDialogElement.prototype.close = function close() {
  this.open = false
  this.dispatchEvent(new Event('close'))
}
