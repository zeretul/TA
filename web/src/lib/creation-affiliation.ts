/**
 * The creation page offers an affiliation and a starting place, and which ones
 * are legal depends on the origin the player picked. The server enforces the
 * same rules (internal/storypack/identity.go); these helpers only keep the page
 * from offering a combination the server would refuse, so a player never has to
 * discover the conflict by being rejected.
 */
import type {
  StoryPackCreationLocation,
  StoryPackCreationOption,
  StoryPackCreationOrigin,
} from './api-client/story-packs'

/**
 * The affiliation kind an origin allows: `none` for a loose cultivator, `sect`
 * for a sect disciple, `family` for a family heir. An origin that states no kind
 * (a custom origin) does not constrain the choice.
 */
export function affiliationKindForOrigin(
  origins: StoryPackCreationOrigin[] | undefined,
  originId: string,
  stateBasisTemplateId?: string,
): string {
  const origin = (origins ?? []).find((entry) => entry.id === originId)
  if (origin?.affiliation_kind) return origin.affiliation_kind
  // Custom prose still starts from an authored state basis. Its affiliation
  // must follow that basis, just as the server's identity validator does.
  const templateId = (originId === 'custom' ? stateBasisTemplateId : undefined)
    || origin?.template_id || origin?.default_template_id
  if (!templateId) return ''
  return (origins ?? []).find((entry) => entry.affiliation_kind
    && (entry.template_id === templateId || entry.default_template_id === templateId))?.affiliation_kind ?? ''
}

/** The affiliations the picked origin can actually belong to. */
export function affiliationOptionsFor(
  origins: StoryPackCreationOrigin[] | undefined,
  affiliations: StoryPackCreationOption[] | undefined,
  originId: string,
  stateBasisTemplateId?: string,
): StoryPackCreationOption[] {
  const all = affiliations ?? []
  const kind = affiliationKindForOrigin(origins, originId, stateBasisTemplateId)
  if (!kind) return all
  if (kind === 'family' && all.some(option => option.id === 'own')) return all.filter(option => option.id === 'own')
  return all.filter((option) => (option.kind ?? '') === kind)
}

/**
 * The affiliation a freshly picked origin should start on. A family heir starts
 * on their own household, whose name follows the surname they type, rather than
 * on a family this world already has.
 */
export function defaultAffiliationFor(
  origins: StoryPackCreationOrigin[] | undefined,
  affiliations: StoryPackCreationOption[] | undefined,
  originId: string,
  stateBasisTemplateId?: string,
): string {
  const options = affiliationOptionsFor(origins, affiliations, originId, stateBasisTemplateId)
  if (options.length === 0) return ''
  if (affiliationKindForOrigin(origins, originId, stateBasisTemplateId) === 'family') {
    const own = options.find((option) => option.id === 'own')
    if (own) return own.id
  }
  return options.find((option) => option.default)?.id ?? options[0].id
}

/** The faction a faction-approach place belongs to ("yue.yan.approach" → "yue.yan"). */
function approachFactionID(placeID: string): string {
  const suffix = '.approach'
  return placeID.endsWith(suffix) ? placeID.slice(0, -suffix.length) : ''
}

/**
 * The two-character surnames a player's name may start with. Without the list
 * 欧阳明 would show 阳氏, which reads as somebody else's house. This must stay
 * in step with compoundSurnames in internal/storypack/family.go — the server
 * derives the name that is actually saved, and this is only the page's preview.
 */
const COMPOUND_SURNAMES = [
  '欧阳', '太史', '端木', '上官', '司马', '东方', '独孤', '南宫', '万俟',
  '闻人', '夏侯', '诸葛', '尉迟', '公羊', '赫连', '澹台', '皇甫', '宗政',
  '濮阳', '公冶', '太叔', '申屠', '公孙', '慕容', '仲孙', '钟离', '长孙',
  '宇文', '司徒', '鲜于', '司空', '闾丘', '子车', '亓官', '司寇', '巫马',
  '公西', '颛孙', '壤驷', '公良', '漆雕', '乐正', '宰父', '谷梁', '拓跋',
  '夹谷', '轩辕', '令狐', '段干', '百里', '呼延', '东郭', '南门', '羊舌',
  '微生', '梁丘', '左丘', '东门', '西门', '第五',
]

/**
 * The household a player's own surname belongs to, in this world's wording
 * (叶流云 → 叶氏, 欧阳明 → 欧阳氏). An empty name has no household yet, so the
 * page keeps the option's own label and asks for a name first.
 */
export function deriveFamilyName(playerName: string): string {
  const name = playerName.trim()
  if (!name) return ''
  const compound = COMPOUND_SURNAMES.find((surname) => name.startsWith(surname))
  const surname = compound ?? Array.from(name)[0] ?? ''
  return surname ? `${surname}氏` : ''
}

/**
 * The affiliation options as the page should label them. The "own household"
 * option is named after the surname the player typed, so the choice reads as
 * theirs before it is submitted.
 */
export function labelAffiliationOptions(
  options: StoryPackCreationOption[],
  playerName: string,
): StoryPackCreationOption[] {
  const family = deriveFamilyName(playerName)
  if (!family) return options
  return options.map((option) => (option.id === 'own' ? { ...option, name: family } : option))
}

/**
 * The places a character with this affiliation can start at. A loose cultivator
 * may be anywhere; anyone else cannot be introduced at another faction's gate or
 * in another country, which is what the server's two location checks refuse.
 */
export function locationOptionsFor(
  locations: StoryPackCreationLocation[] | undefined,
  affiliations: StoryPackCreationOption[] | undefined,
  affiliationId: string,
): StoryPackCreationLocation[] {
  const all = locations ?? []
  const affiliation = (affiliations ?? []).find((option) => option.id === affiliationId)
  if (!affiliation || affiliation.id === 'none' || affiliation.id === 'auto') return all
  const affiliationCountry = (affiliation.country ?? '').trim()
  return all.filter((place) => {
    if (place.id === 'auto') return true
    const faction = approachFactionID(place.id)
    if (faction && faction !== affiliation.id) return false
    const placeCountry = (place.country ?? '').trim()
    if (affiliationCountry && placeCountry && placeCountry !== affiliationCountry) return false
    return true
  })
}

/** The place a freshly picked affiliation should start at. */
export function defaultLocationFor(
  locations: StoryPackCreationLocation[] | undefined,
  affiliations: StoryPackCreationOption[] | undefined,
  affiliationId: string,
): string {
  const options = locationOptionsFor(locations, affiliations, affiliationId)
  if (options.length === 0) return ''
  const marked = options.find((place) => place.default)
  if (marked) return marked.id
  const concrete = options.find((place) => place.id !== 'auto')
  return (concrete ?? options[0]).id
}

/**
 * Whether a place is still offered after the affiliation changed. The page uses
 * this to decide between keeping the player's place and falling back to one the
 * new affiliation allows.
 */
export function isLocationAllowed(
  locations: StoryPackCreationLocation[] | undefined,
  affiliations: StoryPackCreationOption[] | undefined,
  affiliationId: string,
  locationId: string,
): boolean {
  return locationOptionsFor(locations, affiliations, affiliationId).some((place) => place.id === locationId)
}
