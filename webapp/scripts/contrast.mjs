#!/usr/bin/env node
// WCAG 2.x contrast check for the home-screen palette (design B «Карточки», feature 008).
// Constitution IV (v3.1.0): an own palette needs light + dark versions where every text colour pair
// meets 4.5:1, or 3:1 for large text (≥ 24 px). Run: npm run contrast. Exits 1 on any failure.
//
// Token values mirror src/styles-home.css (.ui-b light / dark), which mirror
// specs/008-home-cards-redesign/design/home-cards.html and home-cards-dark.html.

const palettes = {
  light: {
    ground: '#eef2ef',
    card: '#ffffff',
    ink: '#0f1d17',
    muted: '#56665f',
    accent: '#0e7a55',
    accentInk: '#0b5c40',
    tint: '#ddece7',
    deep: '#093b2a',
    warn: '#9a5b00',
    danger: '#c0352a',
    dangerTint: '#f9e3e0',
  },
  dark: {
    ground: '#0d1210',
    card: '#161d1a',
    ink: '#e9efec',
    muted: '#93a39c',
    accent: '#62a991', // accent text/icons in dark = accentInk
    accentInk: '#62a991',
    tint: '#0d271e',
    deep: '#0a4732',
    warn: '#f0b04a',
    danger: '#ff7a6b',
    dangerTint: 'rgba(255,122,107,.12)', // over ground
  },
}

function parse(c) {
  const m = c.match(/^#([0-9a-f]{6})$/i)
  if (m) {
    const n = parseInt(m[1], 16)
    return { r: n >> 16, g: (n >> 8) & 255, b: n & 255, a: 1 }
  }
  const r = c.match(/^rgba\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*([\d.]+)\s*\)$/i)
  if (r) return { r: +r[1], g: +r[2], b: +r[3], a: +r[4] }
  throw new Error('bad colour ' + c)
}

/** Composites colour c (possibly translucent) over an opaque base. */
function over(c, base) {
  const f = parse(c)
  const b = typeof base === 'string' ? parse(base) : base
  const mix = (x, y) => Math.round(x * f.a + y * (1 - f.a))
  return { r: mix(f.r, b.r), g: mix(f.g, b.g), b: mix(f.b, b.b), a: 1 }
}

function lum({ r, g, b }) {
  const ch = (v) => {
    v /= 255
    return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * ch(r) + 0.7152 * ch(g) + 0.0722 * ch(b)
}

function ratio(fg, bg) {
  const [a, b] = [lum(fg), lum(bg)].sort((x, y) => y - x)
  return (a + 0.05) / (b + 0.05)
}

/** Text pairs used by the design: [label, foreground, background (opaque or composited), large?] */
function pairs(p) {
  const deep = parse(p.deep)
  const statBox = over('rgba(255,255,255,.10)', deep)
  const dangerBlock = over(p.dangerTint, p.ground)
  const white = parse('#ffffff')
  return [
    ['ink on ground (header, tile labels)', parse(p.ink), parse(p.ground)],
    ['muted on ground (date)', parse(p.muted), parse(p.ground)],
    ['ink on card', parse(p.ink), parse(p.card)],
    ['muted on card (limit amounts, tabs)', parse(p.muted), parse(p.card)],
    ['accent on card (links, active tab)', parse(p.accent), parse(p.card)],
    ['warn on card (near-limit amount)', parse(p.warn), parse(p.card)],
    ['danger on card (over-limit amount)', parse(p.danger), parse(p.card)],
    ['white on deep (balance, main card)', white, deep, true],
    ['white 78% on deep (main card captions)', over('rgba(255,255,255,.78)', deep), deep],
    ['white on stat box (per-day value)', white, statBox],
    ['white 78% on stat box (per-day label)', over('rgba(255,255,255,.78)', statBox), statBox],
    ['danger on debt block', parse(p.danger), dangerBlock],
    ['muted on debt block', parse(p.muted), dangerBlock],
    ['accentInk on savings tint', parse(p.accentInk), parse(p.tint)],
    ['muted on savings tint', parse(p.muted), parse(p.tint)],
    ['white on accent tile (primary «Расход»)', white, parse('#0e7a55')],
  ]
}

let failures = 0
for (const [name, p] of Object.entries(palettes)) {
  console.log(`\n${name}`)
  for (const [label, fg, bg, large] of pairs(p)) {
    const r = ratio(fg, bg)
    const need = large ? 3 : 4.5
    const ok = r >= need
    if (!ok) failures++
    console.log(`  ${ok ? 'ok  ' : 'FAIL'}  ${r.toFixed(2).padStart(5)} ≥ ${need}  ${label}`)
  }
}
console.log(failures ? `\n${failures} pair(s) below target` : '\nall pairs pass')
process.exit(failures ? 1 : 0)
