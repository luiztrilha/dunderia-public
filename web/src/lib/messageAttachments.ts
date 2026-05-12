export const MAX_COMPOSER_ATTACHMENTS = 5
export const MAX_ATTACHMENT_BYTES = 64 * 1024

export interface ComposerAttachment {
  id: string
  name: string
  type: string
  size: number
  content: string
  truncated: boolean
  kind?: AttachmentKind
  extraction?: 'full' | 'partial' | 'metadata'
}

export interface AttachmentContextOptions {
  heading: string
}

export type AttachmentKind =
  | 'text'
  | 'pdf'
  | 'document'
  | 'spreadsheet'
  | 'presentation'
  | 'image'
  | 'audio'
  | 'video'
  | 'unknown'

const textExtensions = new Set([
  'bat',
  'cmd',
  'conf',
  'cs',
  'css',
  'csv',
  'go',
  'html',
  'ini',
  'java',
  'js',
  'json',
  'jsx',
  'log',
  'md',
  'ps1',
  'py',
  'rs',
  'sql',
  'toml',
  'ts',
  'tsx',
  'txt',
  'xml',
  'yaml',
  'yml',
])

const textMimeTypes = new Set([
  'application/javascript',
  'application/json',
  'application/typescript',
  'application/x-javascript',
  'application/x-sh',
  'application/x-yaml',
  'application/xml',
  'text/markdown',
])

const officeMimeTypes: Record<string, AttachmentKind> = {
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document': 'document',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet': 'spreadsheet',
  'application/vnd.openxmlformats-officedocument.presentationml.presentation': 'presentation',
}

const extensionKinds: Record<string, AttachmentKind> = {
  docx: 'document',
  xlsx: 'spreadsheet',
  pptx: 'presentation',
  pdf: 'pdf',
  png: 'image',
  jpg: 'image',
  jpeg: 'image',
  gif: 'image',
  webp: 'image',
  bmp: 'image',
  svg: 'image',
  mp3: 'audio',
  wav: 'audio',
  m4a: 'audio',
  aac: 'audio',
  flac: 'audio',
  ogg: 'audio',
  opus: 'audio',
  wma: 'audio',
  mp4: 'video',
  mov: 'video',
  mkv: 'video',
  webm: 'video',
  avi: 'video',
  m4v: 'video',
}

function extensionFromName(name: string): string {
  const trimmed = name.trim().toLowerCase()
  const dot = trimmed.lastIndexOf('.')
  return dot >= 0 ? trimmed.slice(dot + 1) : ''
}

export function isSupportedAttachmentFile(file: Pick<File, 'name' | 'type'>): boolean {
  return classifyAttachmentFile(file) !== 'unknown'
}

export function classifyAttachmentFile(file: Pick<File, 'name' | 'type'>): AttachmentKind {
  const type = file.type.trim().toLowerCase()
  if (type.startsWith('text/')) return 'text'
  if (type.startsWith('image/')) return 'image'
  if (type.startsWith('audio/')) return 'audio'
  if (type.startsWith('video/')) return 'video'
  if (type === 'application/pdf') return 'pdf'
  if (officeMimeTypes[type]) return officeMimeTypes[type]
  if (textMimeTypes.has(type)) return 'text'

  const extension = extensionFromName(file.name)
  if (textExtensions.has(extension)) return 'text'
  return extensionKinds[extension] ?? 'unknown'
}

export function formatAttachmentSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  if (bytes < 1024) return `${Math.round(bytes)} B`
  const kb = bytes / 1024
  if (kb < 1024) return `${kb.toFixed(kb >= 10 ? 0 : 1)} KB`
  const mb = kb / 1024
  return `${mb.toFixed(mb >= 10 ? 0 : 1)} MB`
}

export function truncateAttachmentContent(content: string, maxChars = MAX_ATTACHMENT_BYTES): { content: string; truncated: boolean } {
  if (content.length <= maxChars) {
    return { content, truncated: false }
  }
  return { content: content.slice(0, maxChars), truncated: true }
}

function normalizeAttachmentContent(content: string): string {
  return content.replace(/\r\n/g, '\n').replace(/\r/g, '\n').replace(/```/g, '` ` `').trimEnd()
}

function safeAttachmentName(name: string): string {
  const normalized = name.replace(/[\r\n\t]/g, ' ').replace(/[\\/]+/g, '_').replace(/\s+/g, ' ').trim()
  return normalized || 'attachment'
}

function decodeXmlEntities(value: string): string {
  return value
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&')
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&#(\d+);/g, (_, code) => String.fromCodePoint(Number(code)))
    .replace(/&#x([0-9a-f]+);/gi, (_, code) => String.fromCodePoint(Number.parseInt(code, 16)))
}

function xmlToText(xml: string): string {
  return decodeXmlEntities(xml)
    .replace(/<[^>]+>/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

function bytesToLatin1(bytes: Uint8Array): string {
  let out = ''
  const chunkSize = 0x8000
  for (let i = 0; i < bytes.length; i += chunkSize) {
    out += String.fromCharCode(...bytes.slice(i, i + chunkSize))
  }
  return out
}

function unescapePdfString(value: string): string {
  return value
    .replace(/\\n/g, '\n')
    .replace(/\\r/g, '\r')
    .replace(/\\t/g, '\t')
    .replace(/\\b/g, '\b')
    .replace(/\\f/g, '\f')
    .replace(/\\([()\\])/g, '$1')
    .replace(/\\([0-7]{1,3})/g, (_, octal) => String.fromCharCode(Number.parseInt(octal, 8)))
}

function extractPdfText(bytes: Uint8Array): string {
  const raw = bytesToLatin1(bytes)
  const parts: string[] = []
  const textOps = raw.matchAll(/\((?:\\.|[^\\)])*\)\s*Tj/g)
  for (const match of textOps) {
    const wrapped = match[0].replace(/\s*Tj$/, '')
    parts.push(unescapePdfString(wrapped.slice(1, -1)))
  }
  const arrayOps = raw.matchAll(/\[(.*?)\]\s*TJ/gs)
  for (const match of arrayOps) {
    const strings = match[1].match(/\((?:\\.|[^\\)])*\)/g) ?? []
    if (strings.length > 0) {
      parts.push(strings.map((item) => unescapePdfString(item.slice(1, -1))).join(''))
    }
  }
  return parts.join('\n').replace(/\s+\n/g, '\n').replace(/\n{3,}/g, '\n\n').trim()
}

async function inflateRaw(data: Uint8Array): Promise<Uint8Array | null> {
  if (typeof DecompressionStream === 'undefined') {
    return null
  }
  const stream = new Blob([data]).stream().pipeThrough(new DecompressionStream('deflate-raw' as CompressionFormat))
  const buffer = await new Response(stream).arrayBuffer()
  return new Uint8Array(buffer)
}

async function readZipTextEntries(bytes: Uint8Array, wanted: (name: string) => boolean): Promise<Record<string, string>> {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  const decoder = new TextDecoder('utf-8')
  const entries: Record<string, string> = {}
  let offset = 0

  while (offset + 30 <= bytes.length) {
    if (view.getUint32(offset, true) !== 0x04034b50) break
    const flags = view.getUint16(offset + 6, true)
    const method = view.getUint16(offset + 8, true)
    const compressedSize = view.getUint32(offset + 18, true)
    const nameLength = view.getUint16(offset + 26, true)
    const extraLength = view.getUint16(offset + 28, true)
    const nameStart = offset + 30
    const dataStart = nameStart + nameLength + extraLength
    const name = decoder.decode(bytes.slice(nameStart, nameStart + nameLength))
    if ((flags & 0x08) !== 0 || compressedSize === 0xffffffff || dataStart + compressedSize > bytes.length) {
      break
    }

    const compressed = bytes.slice(dataStart, dataStart + compressedSize)
    if (wanted(name)) {
      let entryBytes: Uint8Array | null = null
      if (method === 0) {
        entryBytes = compressed
      } else if (method === 8) {
        entryBytes = await inflateRaw(compressed)
      }
      if (entryBytes) {
        entries[name] = decoder.decode(entryBytes)
      }
    }
    offset = dataStart + compressedSize
  }

  return entries
}

function wantedOfficeEntry(kind: AttachmentKind, name: string): boolean {
  if (kind === 'document') {
    return /^word\/(document|header\d*|footer\d*)\.xml$/i.test(name)
  }
  if (kind === 'spreadsheet') {
    return /^xl\/sharedStrings\.xml$/i.test(name) || /^xl\/worksheets\/sheet\d+\.xml$/i.test(name)
  }
  if (kind === 'presentation') {
    return /^ppt\/slides\/slide\d+\.xml$/i.test(name)
  }
  return false
}

async function extractOfficeText(file: File, kind: AttachmentKind): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer())
  const entries = await readZipTextEntries(bytes, (name) => wantedOfficeEntry(kind, name))
  return Object.entries(entries)
    .sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true }))
    .map(([name, xml]) => {
      const text = xmlToText(xml)
      return text ? `${name}\n${text}` : ''
    })
    .filter(Boolean)
    .join('\n\n')
    .trim()
}

export function buildMediaAttachmentContent(file: Pick<File, 'name' | 'type' | 'size'>): string {
  return [
    'Media file attached.',
    `Name: ${safeAttachmentName(file.name)}`,
    `Type: ${file.type || 'unknown'}`,
    `Size: ${formatAttachmentSize(file.size)}`,
    'OCR or transcription was not run in this message composer. Ask the operator to run the local media/OCR tools if the actual visual or spoken content is needed.',
  ].join('\n')
}

export async function readComposerAttachment(file: File): Promise<ComposerAttachment> {
  const kind = classifyAttachmentFile(file)
  if (kind === 'unknown') {
    throw new Error('unsupported file type')
  }

  let raw = ''
  let extraction: ComposerAttachment['extraction'] = 'full'
  if (kind === 'text') {
    const sliced = file.size > MAX_ATTACHMENT_BYTES
      ? file.slice(0, MAX_ATTACHMENT_BYTES)
      : file
    raw = await sliced.text()
    if (raw.includes('\u0000')) {
      throw new Error('binary file detected')
    }
  } else if (kind === 'pdf') {
    raw = extractPdfText(new Uint8Array(await file.arrayBuffer()))
    extraction = raw ? 'partial' : 'metadata'
  } else if (kind === 'document' || kind === 'spreadsheet' || kind === 'presentation') {
    raw = await extractOfficeText(file, kind)
    extraction = raw ? 'partial' : 'metadata'
  } else if (kind === 'image' || kind === 'audio' || kind === 'video') {
    raw = buildMediaAttachmentContent(file)
    extraction = 'metadata'
  }

  if (!raw.trim()) {
    raw = [
      'File attached, but no readable text could be extracted in the browser.',
      `Name: ${safeAttachmentName(file.name)}`,
      `Type: ${file.type || 'unknown'}`,
      `Size: ${formatAttachmentSize(file.size)}`,
    ].join('\n')
  }
  const truncated = truncateAttachmentContent(raw, MAX_ATTACHMENT_BYTES)

  return {
    id: `${file.name}-${file.size}-${file.lastModified}-${Math.random().toString(16).slice(2)}`,
    name: safeAttachmentName(file.name),
    type: file.type || 'text/plain',
    size: file.size,
    content: truncated.content,
    truncated: file.size > MAX_ATTACHMENT_BYTES || truncated.truncated,
    kind,
    extraction,
  }
}

export function appendAttachmentContext(
  content: string,
  attachments: ComposerAttachment[],
  options: AttachmentContextOptions,
): string {
  if (attachments.length === 0) return content

  const blocks = attachments.map((attachment, index) => {
    const truncated = attachment.truncated ? ', content truncated' : ''
    const extraction = attachment.extraction ? `, extraction: ${attachment.extraction}` : ''
    const metadata = `${attachment.type || 'text/plain'}, ${formatAttachmentSize(attachment.size)}${extraction}${truncated}`
    return [
      `[${index + 1}] ${safeAttachmentName(attachment.name)} (${metadata})`,
      '```text',
      normalizeAttachmentContent(attachment.content),
      '```',
    ].join('\n')
  })

  return [
    content.trimEnd(),
    '',
    '---',
    options.heading,
    '',
    ...blocks,
  ].join('\n')
}
