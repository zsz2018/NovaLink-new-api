export function monitorGroupLabel(code: string, displayName?: string): string {
  const name = displayName?.trim()
  return name && name !== code ? `${code} (${name})` : code
}
