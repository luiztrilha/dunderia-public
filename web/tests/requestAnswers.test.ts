import assert from 'node:assert/strict'
import {
  canSubmitRequestOption,
  normalizeRequestCustomText,
  requestOptionTextKey,
} from '../src/lib/requestAnswers.ts'

assert.equal(requestOptionTextKey('req-1', 'approve'), 'req-1:approve')
assert.equal(normalizeRequestCustomText('  ship it  '), 'ship it')

assert.equal(canSubmitRequestOption({ requiresText: false }), true)
assert.equal(canSubmitRequestOption({ requiresText: true, customText: '' }), false)
assert.equal(canSubmitRequestOption({ requiresText: true, customText: '  details  ' }), true)
assert.equal(canSubmitRequestOption({ requiresText: true, customText: 'details', busy: true }), false)

console.log('requestAnswers assertions passed')
