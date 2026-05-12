import assert from 'node:assert/strict'
import {
  appendAttachmentContext,
  buildMediaAttachmentContent,
  classifyAttachmentFile,
  formatAttachmentSize,
  truncateAttachmentContent,
  type ComposerAttachment,
} from '../src/lib/messageAttachments.ts'

const attachment: ComposerAttachment = {
  id: 'att-1',
  name: 'notes.md',
  type: 'text/markdown',
  size: 1024,
  content: '# Notes\nUse the existing broker message flow.',
  truncated: false,
}

const message = appendAttachmentContext('Please review this.', [attachment], {
  heading: 'Attached files for context',
})

assert.match(message, /^Please review this\./)
assert.match(message, /Attached files for context/)
assert.match(message, /\[1\] notes\.md \(text\/markdown, 1\.0 KB\)/)
assert.match(message, /```text\n# Notes\nUse the existing broker message flow\.\n```/)

const truncated = truncateAttachmentContent('a'.repeat(24), 10)
assert.equal(truncated.content, 'aaaaaaaaaa')
assert.equal(truncated.truncated, true)

const truncatedMessage = appendAttachmentContext('Summarize.', [{
  ...attachment,
  content: truncated.content,
  truncated: truncated.truncated,
}], {
  heading: 'Attached files for context',
})

assert.match(truncatedMessage, /content truncated/)
assert.equal(formatAttachmentSize(1536), '1.5 KB')

assert.equal(classifyAttachmentFile({ name: 'report.pdf', type: 'application/pdf' }), 'pdf')
assert.equal(classifyAttachmentFile({ name: 'brief.docx', type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document' }), 'document')
assert.equal(classifyAttachmentFile({ name: 'numbers.xlsx', type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), 'spreadsheet')
assert.equal(classifyAttachmentFile({ name: 'screen.png', type: 'image/png' }), 'image')
assert.equal(classifyAttachmentFile({ name: 'call.mp3', type: 'audio/mpeg' }), 'audio')
assert.equal(classifyAttachmentFile({ name: 'demo.mp4', type: 'video/mp4' }), 'video')

const mediaContent = buildMediaAttachmentContent({ name: 'screen.png', type: 'image/png', size: 2048 })
assert.match(mediaContent, /Media file attached/)
assert.match(mediaContent, /OCR or transcription was not run/)
assert.match(mediaContent, /screen\.png/)

console.log('messageAttachments assertions passed')
