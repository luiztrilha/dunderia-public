export function extractTaggedSlugs(content: string, knownSlugs: string[]): string[] {
  const known = new Set(
    knownSlugs
      .map((slug) => String(slug || '').trim().toLowerCase())
      .filter(Boolean),
  )
  if (known.size === 0) return []

  const tagged: string[] = []
  const seen = new Set<string>()
  const mentionRe = /@(\S+)/g
  let match: RegExpExecArray | null
  while ((match = mentionRe.exec(content)) !== null) {
    const slug = match[1].toLowerCase().replace(/[^a-z0-9-]/g, '')
    if (!slug || !known.has(slug) || seen.has(slug)) continue
    seen.add(slug)
    tagged.push(slug)
  }
  return tagged
}
