// The generic Settings tab in Admin.vue labels each setting through
// `admin.settings.label.<key>` and describes it through
// `admin.settings.description.<key>`. A missing key degrades to the de-slugged
// setting key and the server's stored English description — plausible in
// English and invisible in review, which is how #305 shipped. These tests pin
// the catalogue to the settings the migrations actually seed.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import en from '../src/locales/en/index.js'

// Rendered elsewhere, not by the generic list: the whole branding category has
// its own tab, and these two keys have bespoke editors with their own copy.
const BESPOKE = new Set(['risk_categories', 'risk_custom_fields'])

// Every (key, category) seeded by an `INSERT INTO settings` in migrations/.
// The key table is a closed registry: organization_settings.setting_key
// REFERENCES settings(key), and only migrations add rows to it. Only INSERTs
// are read: a migration that DELETEs a setting must be taught to this parser,
// or its (now removed) en copy shows up as missing here.
function seededSettings() {
  const dir = new URL('../../migrations/', import.meta.url)
  const rows = []
  for (const file of readdirSync(dir).filter((f) => f.endsWith('.sql')).sort()) {
    const sql = readFileSync(new URL(file, dir), 'utf8')
    for (const stmt of sql.matchAll(/INSERT INTO settings\b[\s\S]*?;[ \t]*$/gm)) {
      for (const m of stmt[0].matchAll(/\(\s*'([a-z0-9_]+)',\s*'(?:[^']|'')*',\s*'([a-z_]+)'/g)) {
        rows.push({ key: m[1], category: m[2] })
      }
    }
  }
  return rows
}

const generic = seededSettings()
  .filter((s) => s.category !== 'branding' && !BESPOKE.has(s.key))
  .map((s) => s.key)

test('the migration parser finds the seeded settings', () => {
  // Guards the regex, not the catalogue: if it silently matched nothing, every
  // other test here would pass vacuously.
  assert.ok(generic.length >= 17, `found only ${generic.length} generic settings`)
  assert.ok(generic.includes('ai_enabled'))
  assert.ok(generic.includes('supplier_review_cycle_low'))
})

test('every generic setting has an en label and description', () => {
  const { label = {}, description = {} } = en.admin.settings
  const missing = generic.flatMap((k) => [
    ...(label[k] ? [] : [`label.${k}`]),
    ...(description[k] ? [] : [`description.${k}`]),
  ])
  assert.deepEqual(missing, [])
})

test('the catalogue holds no key for a setting that does not exist', () => {
  // A typo in the catalogue is a label that never renders.
  const seeded = new Set(generic)
  const { label = {}, description = {} } = en.admin.settings
  const stray = [...Object.keys(label), ...Object.keys(description)].filter((k) => !seeded.has(k))
  assert.deepEqual(stray, [])
})

test('ai_enabled is labelled "AI Enabled", not the de-slugged "Ai Enabled"', () => {
  assert.equal(en.admin.settings.label.ai_enabled, 'AI Enabled')
})
