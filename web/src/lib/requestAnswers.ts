export function requestOptionTextKey(requestId: string, optionId: string): string {
  return `${requestId}:${optionId}`
}

export function normalizeRequestCustomText(value: string | undefined): string {
  return (value ?? '').trim()
}

export function canSubmitRequestOption(input: {
  requiresText?: boolean
  customText?: string
  busy?: boolean
}): boolean {
  if (input.busy) return false
  if (!input.requiresText) return true
  return normalizeRequestCustomText(input.customText).length > 0
}
