const baseDomainPattern = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$/
const domainPrefixPattern = /^[a-z0-9-]*$/

export function normalizeBaseDomainInput(value: string): string {
  return value.trim().toLowerCase()
}

export function normalizeDomainPrefixInput(value: string): string {
  return value.trim().toLowerCase()
}

export function validateBaseDomainInput(value: string): string | null {
  const normalized = normalizeBaseDomainInput(value)
  if (normalized === '') {
    return 'base domain is required'
  }
  if (normalized.length > 253) {
    return 'base domain is too long'
  }
  if (!baseDomainPattern.test(normalized)) {
    return 'base domain must be a valid domain name'
  }
  return null
}

export function validateDomainPrefixInput(value: string): string | null {
  const normalized = normalizeDomainPrefixInput(value)
  if (!domainPrefixPattern.test(normalized)) {
    return 'domain prefix may contain only lowercase letters, numbers, and hyphens'
  }
  const label = `${normalized}app`.replace(/^-+|-+$/g, '') || 'app'
  if (label.length > 63) {
    return 'domain prefix is too long'
  }
  return null
}
