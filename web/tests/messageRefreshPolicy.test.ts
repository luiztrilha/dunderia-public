import assert from 'node:assert/strict'
import {
  getChannelFeedRefreshInterval,
  getThreadPanelRefreshInterval,
  getThreadsAppRefreshInterval,
} from '../src/lib/messageRefreshPolicy.ts'

const activeBase = {
  brokerEventsConnected: false,
  isPageVisible: true,
  isWindowFocused: true,
}

assert.equal(
  getChannelFeedRefreshInterval({
    ...activeBase,
    brokerEventsConnected: true,
    hasActiveThread: false,
  }),
  false,
)

assert.equal(
  getChannelFeedRefreshInterval({
    ...activeBase,
    hasActiveThread: true,
  }),
  15_000,
)

assert.equal(
  getThreadPanelRefreshInterval({
    ...activeBase,
    isPageVisible: false,
  }),
  false,
)

assert.equal(
  getThreadPanelRefreshInterval({
    ...activeBase,
    isWindowFocused: false,
  }),
  20_000,
)

assert.equal(
  getThreadsAppRefreshInterval({
    ...activeBase,
    isWindowFocused: false,
  }),
  45_000,
)

console.log('messageRefreshPolicy assertions passed')
