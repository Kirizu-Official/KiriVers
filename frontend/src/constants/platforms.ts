/**
 * Frontend copy of Go `internal/platform` PresetOS / PresetArch.
 * Comboboxes union these with the live catalog so the dropdown still works if catalog fetch fails.
 */

/** Mirrors `platform.PresetOS`. */
export const COMMON_OS = [
  'windows', 'macos', 'linux', 'freebsd', 'openbsd', 'netbsd', 'dragonfly',
  'solaris', 'illumos', 'aix', 'haiku',
  'android', 'ios', 'ipados', 'tvos', 'watchos', 'visionos', 'harmonyos', 'chromeos',
  'embedded', 'rtos', 'zephyr', 'freertos', 'wasm', 'fuchsia',
] as const

/** Mirrors `platform.PresetArch`. */
export const COMMON_ARCH = [
  'x86', 'x86_64', 'arm', 'armv6', 'armv7', 'arm64',
  'riscv32', 'riscv64', 'loongarch64',
  'mips', 'mipsel', 'mips64', 'mips64el',
  'ppc', 'ppc64', 'ppc64le', 's390x', 'sparc64',
  'wasm32', 'thumb', 'universal', 'any',
] as const

/** Same charset as Go `ValidOSArchSlug` / `osArchSlugRE`. */
export const OS_ARCH_SLUG_PATTERN = /^[a-z0-9_-]{3,64}$/

export function isOsArchSlug (value: string): boolean {
  return OS_ARCH_SLUG_PATTERN.test(value)
}
