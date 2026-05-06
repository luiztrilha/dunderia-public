import assert from 'node:assert/strict'
import {
  dispatchChannelMessagesRefresh,
  subscribeChannelMessagesRefresh,
} from '../src/lib/messageEvents.ts'

const eventTarget = new EventTarget()
Object.defineProperty(globalThis, 'window', {
  value: eventTarget,
  configurable: true,
})

let refreshCount = 0
let lastForceFull: boolean | undefined

const unsubscribe = subscribeChannelMessagesRefresh('general', (detail) => {
  refreshCount += 1
  lastForceFull = detail.forceFull
})

dispatchChannelMessagesRefresh('other', { forceFull: true })
assert.equal(refreshCount, 0)

dispatchChannelMessagesRefresh('general', { forceFull: true })
assert.equal(refreshCount, 1)
assert.equal(lastForceFull, true)

unsubscribe()
dispatchChannelMessagesRefresh('general', { forceFull: true })
assert.equal(refreshCount, 1)

console.log('messageEvents assertions passed')
