/**
 * The aptitude line as a player reads it.
 *
 * The line is written once, when the character is created, and then stored as a
 * settled state value: the cultivation rules read the tier word back out of it
 * every turn. Characters made before the wording changed still carry the old
 * phrasing, which read like a note about what the rules would allow —
 * "修行资质普通。一般悟性一般。兵器方向偏拳脚，只是经历与机会的倾向，不代表
 * 已学会对应高阶技法。" — instead of a sentence about the character.
 *
 * Rewriting the stored value is not an option: it is a canonical record, and
 * changing it would change what the rules read. So the rewrite happens here,
 * at render time only, and only when the old markers are actually present. The
 * tier words themselves are never touched, so the line still says the same
 * character as the stored one does.
 */

const LEGACY_APTITUDE = /[；;]\s*修行资质\s*/
const LEGACY_INSIGHT = /[。.]\s*一般悟性\s*/
const LEGACY_WEAPON = /[。.]\s*兵器方向偏\s*([^，,。]*)[，,]\s*只是经历与机会的倾向[，,]\s*不代表已学会对应高阶技法\s*。?/
const TRAILING_EMPTY = /[，,]\s*。\s*$/

/**
 * The state field the aptitude line lives in.
 *
 * The name is the one the pack registers, and it is what the ledger has to
 * match on: a character's own value is stored under this key, and the rules
 * read it back from the same key.
 */
export const FOUNDATION_FIELD_NAME = '灵根与特殊体质'

/**
 * Rewrites the old aptitude phrasing into the current one.
 *
 * A value that carries none of the old markers is returned unchanged, which is
 * what keeps a line the narrator wrote in its own words from being rewritten
 * into a template.
 */
export function foundationDisplayText(value: string): string {
  const text = value.trim()
  if (!text) return text
  if (!LEGACY_APTITUDE.test(text) && !LEGACY_INSIGHT.test(text) && !LEGACY_WEAPON.test(text)) {
    return text
  }
  let rewritten = text
    .replace(LEGACY_APTITUDE, '，资质')
    .replace(LEGACY_INSIGHT, '，悟性')
    .replace(LEGACY_WEAPON, (_, weapon: string) => `，偏好${weapon.trim()}`)
  rewritten = rewritten.replace(TRAILING_EMPTY, '。')
  return /[。.]$/.test(rewritten) ? rewritten : `${rewritten}。`
}
