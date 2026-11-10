import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, ArrowLeft, Check, Dices, Loader2, Pencil, RefreshCw, Sparkles } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  draftStoryPackIdentity,
  type StoryPackCreation,
  type StoryPackCreationOption,
  type StoryPackIdentity,
  type StoryPackIdentityChoice,
  type StoryPackSummary,
} from '@/lib/api-client'
import { isAgeInRange, normalizeAgeRange, rollAge } from '@/lib/creation-age'
import {
  affiliationKindForOrigin,
  affiliationOptionsFor,
  defaultAffiliationFor,
  labelAffiliationOptions,
} from '@/lib/creation-affiliation'
import { originCopy, visibleCreationOrigins } from '@/lib/pack-origin-copy'
import { cn } from '@/lib/utils'
import { ModelProfileSwitcher } from '@/components/Chat/ModelProfileSwitcher'
import { useConversationConfig } from '@/features/conversation-config/use-conversation-config'

interface CharacterCreationDialogProps {
  open: boolean
  projectId?: string
  pack: StoryPackSummary | null
  onOpenChange: (open: boolean) => void
  /** Starts the story as the confirmed character. Nothing is created before this. */
  onStart: (choice: StoryPackIdentityChoice) => void | Promise<void>
}

const inputCls =
  'nova-field w-full rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-field-focus-border)] focus:bg-[var(--nova-surface-3)]'

type Step = 'choose' | 'preview'

/**
 * The creation page: one screen of choices, then one preview of the person those
 * choices describe.
 *
 * It deliberately owns no character. The draft endpoint resolves the choices
 * against the pack and asks the model only for prose; this component holds the
 * choices and the returned draft, and nothing here is written anywhere until
 * the player presses 开始故事, which goes through the ordinary story start.
 */
export function CharacterCreationDialog({ open, pack, projectId, onOpenChange, onStart }: CharacterCreationDialogProps) {
  const { t } = useTranslation()
  const conversationConfig = useConversationConfig(open && projectId ? { mode: 'interactive', project_id: projectId } : undefined)
  const creation = pack?.creation
  const draftGeneration = useRef(0)
  const draftAbort = useRef<AbortController | null>(null)
  const [step, setStep] = useState<Step>('choose')
  const [name, setName] = useState('')
  const [gender, setGender] = useState('')
  // The age is the player's own number from the moment the page opens: rolled
  // locally inside the pack's range, editable, and never rerolled by a draft,
  // a retry or a rewrite. The server only rolls when the field arrives empty.
  const [ageText, setAgeText] = useState('')
  const [originId, setOriginId] = useState('')
  const [originText, setOriginText] = useState('')
  const [stateBasis, setStateBasis] = useState('')
  const [affiliationId, setAffiliationId] = useState('')
  const [affiliationText, setAffiliationText] = useState('')
  const [openingText, setOpeningText] = useState('')
  const [rootCategoryId, setRootCategoryId] = useState('')
  const [rootAttributes, setRootAttributes] = useState<string[]>([])
  // A variant root replaces the ordinary one rather than adding to it: the two
  // answer the same question, so picking one clears the other.
  const [variantRootId, setVariantRootId] = useState('')
  const [aptitudeId, setAptitudeId] = useState('')

  const [weaponId, setWeaponId] = useState('')
  const [adventureClassId, setAdventureClassId] = useState('')
  const [adventureWeaponId, setAdventureWeaponId] = useState('')
  const [adventureMethodId, setAdventureMethodId] = useState('')
  const [traitIds, setTraitIds] = useState<string[]>([])
  // Empty means no talent, which is a real answer the rules price as nothing.
  const [talentId, setTalentId] = useState('')
  const [extraOpen, setExtraOpen] = useState(false)
  const [identity, setIdentity] = useState('')
  const [appearance, setAppearance] = useState('')
  const [background, setBackground] = useState('')
  const [situation, setSituation] = useState('')
  const [predicament, setPredicament] = useState('')
  const [draft, setDraft] = useState<StoryPackIdentity | null>(null)
  const [contacts, setContacts] = useState<string[]>([])
  const [busy, setBusy] = useState<'' | 'draft' | 'rewrite' | 'start'>('')
  const [error, setError] = useState('')

  const defaults = useMemo(() => creationDefaults(creation), [creation])
  const ageRange = useMemo(() => normalizeAgeRange(creation?.age_range), [creation])

  useEffect(() => {
    // A draft belongs to the world and dialog that requested it. Late model
    // responses may finish, but cannot replace a new world's confirmed choices.
    draftGeneration.current++
    draftAbort.current?.abort()
    draftAbort.current = null
    if (!open) return
    // The affiliation and the place both follow the origin the page opens on,
    // so the first screen is already a legal combination.
    const openingAffiliation = defaultAffiliationFor(creation?.origins, creation?.affiliations, defaults.origin, defaults.stateBasis)
    setStep('choose')
    setName('')
    setGender(defaults.gender)
    setAgeText(String(rollAge(ageRange)))
    setOriginId(defaults.origin)
    setOriginText('')
    setStateBasis(defaults.stateBasis)
    setAffiliationId(openingAffiliation)
    setAffiliationText('')
    setOpeningText('')
    setRootCategoryId(defaults.rootCategory)
    setRootAttributes([])
    setVariantRootId('')
    setAptitudeId(defaults.aptitude)
    setWeaponId(defaults.weapon)
    setAdventureClassId(defaults.adventureClass)
    setAdventureWeaponId(defaults.adventureWeapon)
    setAdventureMethodId(defaults.adventureMethod)
    setTraitIds([])
    setTalentId('')
    setExtraOpen(false)
    setIdentity('')
    setAppearance('')
    setBackground('')
    setSituation('')
    setPredicament('')
    setDraft(null)
    setContacts([])
    setBusy('')
    setError('')
    return () => {
      draftGeneration.current++
      draftAbort.current?.abort()
      draftAbort.current = null
    }
  }, [open, defaults, creation, ageRange])

  if (!pack || !creation) return null

  const basisTemplates = (creation.custom_state_basis_templates?.length ? creation.custom_state_basis_templates : creation.state_basis_templates ?? []).map((id) => ({
    id,
    name: pack.protagonist_templates?.find((template) => template.id === id)?.name ?? id,
  }))
  const arity = rootArity(rootCategoryId)
  // A variant root answers the root question on its own, so its category and
  // attribute count are not asked of the player at all.
  const rootReady = variantRootId !== '' || arity === null || rootAttributes.length === arity
  const age = Number.parseInt(ageText, 10)
  const ageValid = isAgeInRange(ageText, ageRange)
  const canDraft = name.trim().length > 0 && Boolean(originId) && rootReady && ageValid && !conversationConfig.saving

  // The origin decides which affiliations and places are legal, so both lists
  // are derived from it rather than from the pack's full catalog. The server
  // refuses the same combinations; this only keeps the page from offering one.
  const affiliationOptions = labelAffiliationOptions(
    affiliationOptionsFor(creation.origins, creation.affiliations, originId, stateBasis),
    name,
  )
  const affiliationKind = affiliationKindForOrigin(creation.origins, originId, stateBasis)
  const canCustomizeAffiliation = affiliationKind === 'sect'
  const hasAdventureTraining = Boolean(creation.adventure_classes?.length || creation.adventure_weapons?.length || creation.adventure_methods?.length)
  const hasTalents = Boolean(creation.root_categories?.length || creation.variant_roots?.length || creation.aptitudes?.length || creation.weapon_directions?.length || creation.traits?.length || creation.talents?.length)

  const alignAffiliation = (nextOrigin: string, nextBasis: string) => {
    const nextKind = affiliationKindForOrigin(creation.origins, nextOrigin, nextBasis)
    if (nextKind !== affiliationKind) setAffiliationText('')
    const allowed = affiliationOptionsFor(creation.origins, creation.affiliations, nextOrigin, nextBasis)
    const nextAffiliation = allowed.some((option) => option.id === affiliationId)
      ? affiliationId : defaultAffiliationFor(creation.origins, creation.affiliations, nextOrigin, nextBasis)
    setAffiliationId(nextAffiliation)
  }

  const chooseOrigin = (next: string) => {
    setOriginId(next)
    alignAffiliation(next, stateBasis)
  }

  const chooseStateBasis = (next: string) => {
    setStateBasis(next)
    alignAffiliation(originId, next)
  }

  const chooseAffiliation = (next: string) => {
    setAffiliationId(next)
    // The place has to belong to the new affiliation's country and, for a
    // faction gate, to that faction.
  }

  const choice = (): StoryPackIdentityChoice => ({
    name: name.trim(),
    gender,
    age,
    origin_id: originId,
    origin_text: originText.trim() || undefined,
    state_basis_template_id: originId === 'custom' ? stateBasis || undefined : undefined,
    affiliation_id: affiliationId,
    affiliation_text: canCustomizeAffiliation ? affiliationText.trim() || undefined : undefined,

    opening: openingText.trim() || undefined,
    opening_mode: openingText.trim() ? 'custom' : 'ai',
    location_id: 'auto',
    root_category_id: rootCategoryId,
    root_attributes: variantRootId ? undefined : rootAttributes,
    variant_root_id: variantRootId || undefined,
    aptitude_id: aptitudeId,

    weapon_direction_id: weaponId || undefined,
    adventure_class_id: adventureClassId || undefined,
    adventure_weapon_id: adventureWeaponId || undefined,
    adventure_method_id: adventureMethodId || undefined,
    trait_ids: traitIds,
    talent_id: talentId || undefined,
    identity: identity.trim() || undefined,
    appearance: appearance.trim() || undefined,
    background: background.trim() || undefined,
    situation: situation.trim() || undefined,
    predicament: predicament.trim() || undefined,
  })

  const requestDraft = async (rewrite: boolean) => {
    if (busy) return
    const generation = ++draftGeneration.current
    const controller = new AbortController()
    draftAbort.current = controller
    setBusy(rewrite ? 'rewrite' : 'draft')
    setError('')
    try {
      const result = await draftStoryPackIdentity(pack.id, choice(), rewrite, controller.signal)
      if (generation !== draftGeneration.current) return
      setContacts(result.contact_candidates ?? [])
      // The server echoes the age it resolved. It is the same number that was
      // sent, so this only keeps the page honest if a future pack rejects one.
      if (result.identity.age > 0) setAgeText(String(result.identity.age))
      if (result.error) {
        // The draft did not finish. Keep the player on the choice page with
        // everything they typed, and say why.
        setError(result.error)
        return
      }
      setDraft(result.identity)
      setStep('preview')
    } catch (reason) {
      // The page keeps everything the player typed; only the failure is shown.
      if (generation === draftGeneration.current) setError(reason instanceof Error ? reason.message : String(reason))
    } finally {
      if (generation === draftGeneration.current) {
        draftAbort.current = null
        setBusy('')
      }
    }
  }

  const requestReroll = () => {
    // Local only: a new number in the same range, immediately. Nothing is sent
    // anywhere and the identity is not touched, so this works before the page
    // has generated anything.
    setAgeText(String(rollAge(ageRange, Number.parseInt(ageText, 10))))
  }

  const start = async () => {
    if (busy || !draft) return
    const generation = ++draftGeneration.current
    setBusy('start')
    setError('')
    try {
      // The start is the confirmed character, not just the options behind it:
      // the prose the player saw on the preview is what the story's persona
      // fields are built from. The server resolves the numbers again from the
      // pack, so sending the draft's text back cannot change what it is worth.
      await onStart({
        ...choice(),
        location_id: creation.custom_location_policy === 'authored_only' && originId === 'custom' ? draft.location_id : 'auto',
        location_text: creation.custom_location_policy === 'authored_only' && originId === 'custom' ? undefined : draft.location_name || undefined,
        identity: draft.identity_text || undefined,
        appearance: draft.appearance || undefined,
        background: draft.background || undefined,
        situation: draft.situation || undefined,
        predicament: draft.predicament || undefined,
        contacts: draft.contacts && draft.contacts.length > 0 ? draft.contacts : undefined,
        opening: draft.opening || undefined,
      })
    } catch (reason) {
      if (generation === draftGeneration.current) setError(reason instanceof Error ? reason.message : String(reason))
    } finally {
      if (generation === draftGeneration.current) setBusy('')
    }
  }

  const toggleRootAttribute = (id: string) => {
    setRootAttributes((current) => {
      if (current.includes(id)) return current.filter((value) => value !== id)
      if (arity !== null && current.length >= arity) return current
      return [...current, id]
    })
  }

  // Choosing a variant root drops the ordinary root the page was holding, and
  // choosing an ordinary one drops the variant. The two describe the same thing,
  // so leaving the other set would send a request the server refuses.
  const chooseVariantRoot = (id: string) => {
    const next = variantRootId === id ? '' : id
    setVariantRootId(next)
    if (next) setRootAttributes([])
  }

  const chooseRootCategory = (next: string) => {
    setRootCategoryId(next)
    setRootAttributes([])
    setVariantRootId('')
  }

  const toggleTrait = (id: string) => {
    setTraitIds((current) => {
      if (current.includes(id)) return current.filter((value) => value !== id)
      if (current.length >= 3) return current
      return [...current, id]
    })
  }

  return (
    <Dialog open={open} onOpenChange={(next) => { if (busy !== 'start') onOpenChange(next) }}>
      <DialogContent className="flex max-h-[calc(100dvh-2rem)] max-w-[min(calc(100vw-2rem),42rem)] flex-col gap-0 overflow-hidden p-0 max-md:overflow-hidden">
        <DialogHeader className="shrink-0 border-b px-4 py-4 pr-12">
          <DialogTitle>{step === 'choose' ? t('creation.title') : t('creation.previewTitle')}</DialogTitle>
          <DialogDescription>
            {step === 'choose' ? t('creation.subtitle', { world: pack.name }) : t('creation.previewSubtitle')}
          </DialogDescription>
        </DialogHeader>
        {projectId ? <div className="flex items-center gap-2 border-b px-4 py-2 text-xs text-muted-foreground">
          <span>{t('creation.model')}</span>
          <ModelProfileSwitcher agentKey="interactive_story" conversationConfig={conversationConfig} disabled={Boolean(busy)} />
        </div> : null}

        {error ? (
          <div role="alert" className="flex items-start gap-2 rounded-[var(--nova-radius)] mx-4 mt-3 max-h-28 shrink-0 overflow-y-auto border border-[var(--nova-danger)]/30 bg-[var(--nova-danger)]/5 px-3 py-2 text-xs text-[var(--nova-danger)]">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span className="min-w-0 break-words">{error}</span>
          </div>
        ) : null}

        {step === 'choose' ? (
          <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
            <div className="min-h-0 flex-1 overflow-y-auto p-4" data-testid="creation-scroll-body">
            <fieldset disabled={Boolean(busy)} className="min-w-0 space-y-4">
            <section className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label className="flex min-w-0 flex-col gap-1">
                <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('creation.name')}</span>
                <Input
                  value={name}
                  maxLength={24}
                  placeholder={t('creation.namePlaceholder')}
                  onChange={(event) => setName(event.target.value)}
                  className={inputCls}
                />
              </label>
              <div className="flex min-w-0 flex-col gap-1">
                <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('creation.age')}</span>
                <div className="flex min-w-0 items-center gap-2">
                  <Input
                    value={ageText}
                    inputMode="numeric"
                    aria-invalid={ageText.length > 0 && !ageValid}
                    aria-label={t('creation.age')}
                    onChange={(event) => setAgeText(event.target.value.replace(/[^\d]/g, '').slice(0, 3))}
                    className={cn(inputCls, 'min-w-[4rem] text-center tabular-nums')}
                  />
                  <Button
                    type="button"
                    size="xs"
                    variant="ghost"
                    onClick={requestReroll}
                    title={t('creation.rerollAge')}
                  >
                    <Dices data-icon="inline-start" />
                    {t('creation.rerollAge')}
                  </Button>
                </div>
                <span
                  className={cn(
                    'text-[10px] leading-4',
                    ageText.length > 0 && !ageValid ? 'text-[var(--nova-danger)]' : 'text-[var(--nova-text-faint)]',
                  )}
                >
                  {ageText.length > 0 && !ageValid
                    ? t('creation.ageInvalid', { min: ageRange[0], max: ageRange[1] })
                    : t('creation.ageHint', { min: ageRange[0], max: ageRange[1] })}
                </span>
              </div>
            </section>

            <section className="flex min-w-0 flex-col gap-1.5">
              <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('creation.gender')}</span>
              <div role="radiogroup" className="flex flex-wrap gap-1.5">
                {(creation.genders ?? []).map((option) => (
                  <OptionChip
                    key={option.id}
                    selected={gender === option.id}
                    onClick={() => setGender(option.id)}
                    label={option.name}
                  />
                ))}
              </div>
            </section>

            <section className="flex min-w-0 flex-col gap-1.5">
              <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('creation.origin')}</span>
              <div role="radiogroup" className="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
                {visibleCreationOrigins(creation).map((option) => (
                  <button
                    key={option.id}
                    type="button"
                    role="radio"
                    aria-checked={originId === option.id}
                    onClick={() => chooseOrigin(option.id)}
                    className={cn(
                      'min-w-0 rounded-[var(--nova-radius)] border px-2.5 py-2 text-left transition-colors',
                      originId === option.id
                        ? 'border-[var(--nova-accent)] bg-[var(--nova-accent)]/10'
                        : 'border-[var(--nova-border)] hover:border-[var(--nova-text-faint)]',
                    )}
                  >
                    <span className="flex items-center gap-1 text-xs font-medium">
                      {originId === option.id ? <Check className="h-3 w-3 shrink-0" /> : null}
                      {option.name}
                      {option.opening_tier ? <span className="text-[10px] text-muted-foreground">{t(option.opening_tier === 'strong' ? 'worldPack.overview.strongOrigin' : 'worldPack.overview.ordinaryOrigin')}</span> : null}
                    </span>
                    {option.description ? (
                      <span className="mt-1 block text-xs leading-5 text-[var(--nova-text-muted)]">
                        {originCopy(option.description, t(option.affiliation_kind === 'family' ? 'creation.familyPlaceholderName' : 'creation.affiliationPlaceholderName'))}
                      </span>
                    ) : null}
                  </button>
                ))}
              </div>
              {originId === 'custom' ? (
                <div className="mt-1 flex flex-col gap-2">
                  <Textarea
                    value={originText}
                    maxLength={120}
                    rows={2}
                    placeholder={t('creation.originPlaceholder')}
                    onChange={(event) => setOriginText(event.target.value)}
                    className={inputCls}
                  />
                  {basisTemplates.length > 1 ? (
                    <label className="flex min-w-0 flex-col gap-1">
                      <span className="text-[10px] text-[var(--nova-text-faint)]">{t('creation.stateBasis')}</span>
                      <select
                        value={stateBasis}
                        onChange={(event) => chooseStateBasis(event.target.value)}
                        className={inputCls}
                      >
                        {basisTemplates.map((template) => (
                          <option key={template.id} value={template.id}>{template.name}</option>
                        ))}
                      </select>
                    </label>
                  ) : null}
                </div>
              ) : null}
            </section>

            <section className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {/* A loose cultivator belongs to nothing, so there is no choice to
                  offer; showing a one-item dropdown would suggest otherwise. */}
              {affiliationKind === 'family' && affiliationOptions.length === 1 ? <div className="flex flex-col gap-1 text-xs"><span className="text-muted-foreground">{t('creation.affiliation')}</span><span>{affiliationOptions[0].name}</span></div> : null}
              {affiliationOptions.length <= 1 ? null : (
                <OptionSelect
                  label={t('creation.affiliation')}
                  value={affiliationId}
                  options={affiliationOptions}
                  onChange={chooseAffiliation}
                  hint={t(`creation.affiliationHint.${affiliationKind || 'any'}`)}
                />
              )}
            </section>

            <section className="grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2">
              {canCustomizeAffiliation ? <label className="flex min-w-0 flex-col gap-1 text-xs">{t('creation.customAffiliation')}
                <Input aria-label={t('creation.customAffiliation')} value={affiliationText} maxLength={120} placeholder={t('creation.customAffiliationHint')} onChange={event => setAffiliationText(event.target.value)} />
              </label> : null}
            </section>
            <label className="flex min-w-0 flex-col gap-1 text-xs">{t('creation.customOpening')}
              <Textarea aria-label={t('creation.customOpening')} value={openingText} maxLength={600} rows={3} placeholder={t('creation.customOpeningHint')} onChange={event => setOpeningText(event.target.value)} />
            </label>

            {hasAdventureTraining ? <section className="flex min-w-0 flex-col gap-3 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2.5" aria-label={t('creation.adventureTraining')}>
              <h4 className="text-xs font-medium text-[var(--nova-text-muted)]">{t('creation.adventureTraining')}</h4>
              {creation.adventure_classes?.length ? <OptionSelect label={t('creation.adventureClass')} value={adventureClassId} options={creation.adventure_classes} onChange={setAdventureClassId} hint={creation.adventure_classes.find(option => option.id === adventureClassId)?.description} /> : null}
              {creation.adventure_weapons?.length ? <OptionSelect label={t('creation.adventureWeapon')} value={adventureWeaponId} options={creation.adventure_weapons} onChange={setAdventureWeaponId} hint={creation.adventure_weapons.find(option => option.id === adventureWeaponId)?.description} /> : null}
              {creation.adventure_methods?.length ? <OptionSelect label={t('creation.adventureMethod')} value={adventureMethodId} options={creation.adventure_methods} onChange={setAdventureMethodId} hint={creation.adventure_methods.find(option => option.id === adventureMethodId)?.description} /> : null}
              <p className="text-xs leading-5 text-[var(--nova-text-faint)]">{t('creation.adventureTrainingHint')}</p>
              <p className="text-xs leading-5 text-[var(--nova-text-faint)]">{t('creation.adventureOriginHint')}</p>
            </section> : null}

            {hasTalents ? (
            <section className="flex min-w-0 flex-col gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2.5">
              <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('creation.talents')}</span>

              {/* While a variant is chosen the ordinary root is not a question
                  any more, so its category and chips are hidden rather than
                  left tappable: a selected chip next to a selected variant
                  would read as "both", which the server refuses. */}
              {creation.root_categories?.length && !variantRootId ? <div className="flex min-w-0 flex-col gap-1.5">
                <OptionSelect
                  label={t('creation.rootCategory')}
                  value={rootCategoryId}
                  options={creation.root_categories ?? []}
                  onChange={chooseRootCategory}
                />
                <div className="flex min-w-0 flex-wrap gap-1.5">
                  {(creation.root_attributes ?? []).map((option) => (
                    <OptionChip
                      key={option.id}
                      selected={rootAttributes.includes(option.id)}
                      onClick={() => toggleRootAttribute(option.id)}
                      label={option.name}
                    />
                  ))}
                </div>
                <span className="text-[10px] leading-4 text-[var(--nova-text-faint)]">
                  {t(`creation.effect.${creation.effect_notes?.root ?? 'narrative'}`)}
                  {arity !== null ? ` · ${t('creation.rootArity', { count: arity })}` : ''}
                </span>
              </div> : null}

              {/* A variant root is one pick instead of a category and a count, and
                  picking one clears the ordinary root above. */}
              {creation.variant_roots?.length ? <div className="flex min-w-0 flex-col gap-1.5">
                <span className="text-[10px] text-[var(--nova-text-faint)]">{t('creation.variantRoot')}</span>
                <div className="flex min-w-0 flex-wrap gap-1.5" role="radiogroup" aria-label={t('creation.variantRoot')}>
                  {(creation.variant_roots ?? []).map((option) => (
                    <OptionChip
                      key={option.id}
                      selected={variantRootId === option.id}
                      onClick={() => chooseVariantRoot(option.id)}
                      label={option.name}
                    />
                  ))}
                </div>
                <span className="text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('creation.variantRootHint')}</span>
              </div> : null}

              {creation.aptitudes?.length ? <div className="flex min-w-0 flex-col gap-1.5">
                <OptionSelect
                  label={t('creation.aptitude')}
                  value={aptitudeId}
                  options={creation.aptitudes ?? []}
                  onChange={setAptitudeId}
                />
                <span className="text-[10px] leading-4 text-[var(--nova-text-faint)]">
                  {t(`creation.effect.${creation.effect_notes?.aptitude ?? 'narrative'}`)}
                </span>
              </div> : null}

              {creation.weapon_directions?.length ? (
                <OptionSelect
                  label={t('creation.weapon')}
                  value={weaponId}
                  options={creation.weapon_directions ?? []}
                  onChange={setWeaponId}
                />
              ) : null}

              {creation.traits?.length ? <div className="flex min-w-0 flex-col gap-1.5">
                <span className="text-[10px] text-[var(--nova-text-faint)]">{t('creation.traits')}</span>
                <div className="flex min-w-0 flex-wrap gap-1.5">
                  {(creation.traits ?? []).map((option) => (
                    <OptionChip
                      key={option.id}
                      selected={traitIds.includes(option.id)}
                      onClick={() => toggleTrait(option.id)}
                      label={option.name}
                    />
                  ))}
                </div>
                <span className="text-[10px] leading-4 text-[var(--nova-text-faint)]">
                  {t(`creation.effect.${creation.effect_notes?.trait ?? 'narrative'}`)}
                </span>
              </div> : null}

              {/* One at most, and none is a legal answer: tapping the chosen chip
                  again clears it, so the page never needs a separate 不选 option. */}
              {creation.talents?.length ? <div className="flex min-w-0 flex-col gap-1.5">
                <span className="text-[10px] text-[var(--nova-text-faint)]">{t('creation.innateTalent')}</span>
                <div className="flex min-w-0 flex-wrap gap-1.5" role="radiogroup" aria-label={t('creation.innateTalent')}>
                  {(creation.talents ?? []).map((option) => (
                    <OptionChip
                      key={option.id}
                      selected={talentId === option.id}
                      onClick={() => setTalentId((current) => (current === option.id ? '' : option.id))}
                      label={option.name}
                    />
                  ))}
                </div>
                <span className="text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('creation.innateTalentHint')}</span>
              </div> : null}
            </section>
            ) : null}

            <section className="flex min-w-0 flex-col gap-2">
              <button
                type="button"
                className="flex items-center gap-1 self-start text-xs text-[var(--nova-text-muted)] hover:text-[var(--nova-text)]"
                onClick={() => setExtraOpen((current) => !current)}
              >
                <Pencil className="h-3 w-3" />
                {extraOpen ? t('creation.extraHide') : t('creation.extraShow')}
              </button>
              {extraOpen ? (
                <div className="flex flex-col gap-2">
                  <Textarea
                    value={identity}
                    rows={2}
                    maxLength={600}
                    placeholder={t('creation.identityPlaceholder')}
                    onChange={(event) => setIdentity(event.target.value)}
                    className={inputCls}
                  />
                  <Textarea
                    value={appearance}
                    rows={2}
                    maxLength={600}
                    placeholder={t('creation.appearancePlaceholder')}
                    onChange={(event) => setAppearance(event.target.value)}
                    className={inputCls}
                  />
                  <Textarea
                    value={background}
                    rows={2}
                    maxLength={600}
                    placeholder={t('creation.backgroundPlaceholder')}
                    onChange={(event) => setBackground(event.target.value)}
                    className={inputCls}
                  />
                  <Textarea
                    value={situation}
                    rows={2}
                    maxLength={600}
                    placeholder={t('creation.situationPlaceholder')}
                    onChange={(event) => setSituation(event.target.value)}
                    className={inputCls}
                  />
                  <Textarea
                    value={predicament}
                    rows={2}
                    maxLength={600}
                    placeholder={t('creation.predicamentPlaceholder')}
                    onChange={(event) => setPredicament(event.target.value)}
                    className={inputCls}
                  />
                  <p className="text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('creation.extraHint')}</p>
                </div>
              ) : null}
            </section>

            </fieldset>
            </div>
            <DialogFooter className="mx-0 mb-0 shrink-0 bg-popover">
              <Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={busy === 'start'}>
                {t('common.cancel')}
              </Button>
              <Button type="button" onClick={() => void requestDraft(false)} disabled={!canDraft || Boolean(busy)}>
                {busy === 'draft' ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Sparkles data-icon="inline-start" />}
                {busy === 'draft' ? t('creation.drafting') : t('creation.generate')}
              </Button>
            </DialogFooter>
          </div>
        ) : draft ? (
          <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
            <div className="min-h-0 flex-1 overflow-y-auto p-4" data-testid="creation-scroll-body">
            <fieldset disabled={Boolean(busy)} className="min-w-0 space-y-4">
            <div className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface)] px-3 py-3">
              <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1">
                <h3 className="break-words text-base font-semibold">{draft.name}</h3>
                <span className="text-xs text-[var(--nova-text-muted)]">
                  {[draft.gender_name, t('creation.ageValue', { age: draft.age }), draft.origin_text]
                    .filter(Boolean)
                    .join(' · ')}
                </span>
              </div>
              <p className="mt-1 text-xs text-[var(--nova-text-muted)]">
                {[draft.affiliation_name, draft.location_name].filter(Boolean).join(' · ')}
              </p>
              <label className="mt-2 flex min-w-0 flex-col gap-1 text-xs">{t('creation.customOpening')}
                <Textarea aria-label={t('creation.customOpening')} value={draft.opening || ''} maxLength={600} rows={4} onChange={event => { setOpeningText(event.target.value); setDraft({ ...draft, opening: event.target.value }) }} />
              </label>
              {draft.identity_text ? <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-6">{draft.identity_text}</p> : null}
              {draft.appearance ? <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-[var(--nova-text-muted)]">{draft.appearance}</p> : null}
              {draft.background ? <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-[var(--nova-text-muted)]">{draft.background}</p> : null}
              {draft.situation ? <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-[var(--nova-text-muted)]">{draft.situation}</p> : null}
              {draft.predicament ? (
                <p className="mt-1 text-xs leading-5 text-[var(--nova-text-muted)]">
                  {t('creation.predicamentLabel')}{draft.predicament}
                </p>
              ) : null}
            </div>

            {hasAdventureTraining ? <section className="min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2.5">
              <h4 className="text-xs font-medium text-[var(--nova-text-muted)]">{t('creation.adventureTraining')}</h4>
              <dl className="mt-1.5 flex flex-col gap-1 text-xs">
                <Fact label={t('creation.adventureClass')} value={draft.adventure_class_name} />
                <Fact label={t('creation.adventureWeapon')} value={draft.adventure_weapon_name} />
                <Fact label={t('creation.adventureMethod')} value={draft.adventure_method_name} />
              </dl>
              <p className="mt-2 text-xs leading-5 text-[var(--nova-text-faint)]">{t('creation.adventureTrainingHint')}</p>
            </section> : null}

            <div className={cn('grid grid-cols-1 gap-3', hasTalents && 'sm:grid-cols-2')}>
              {hasTalents ? <section className="min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2.5">
                <h4 className="text-xs font-medium text-[var(--nova-text-muted)]">{t('creation.talents')}</h4>
                <dl className="mt-1.5 flex flex-col gap-1 text-xs">
                  <Fact label={t('creation.root')} value={draft.foundation} />
                  {creation.aptitudes?.length ? <Fact label={t('creation.aptitude')} value={draft.aptitude_name} /> : null}
                  {creation.weapon_directions?.length ? <Fact label={t('creation.weapon')} value={draft.weapon_direction_name} /> : null}
                  {creation.traits?.length ? <Fact label={t('creation.traits')} value={(draft.trait_names ?? []).join(t('creation.listSeparator'))} /> : null}
                  {creation.talents?.length ? (
                    <Fact
                      label={t('creation.innateTalent')}
                      value={draft.talent_name ? [draft.talent_name, draft.talent_bonus].filter(Boolean).join(' · ') : t('creation.noTalent')}
                    />
                  ) : null}
                </dl>
              </section> : null}

              {draft.initial_state ? <section className="min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2.5">
                <h4 className="text-xs font-medium text-[var(--nova-text-muted)]">{t('creation.startingState')}</h4>
                <dl className="mt-1.5 flex flex-col gap-1 text-xs">
                  {draft.initial_state.authored_fields?.length ? draft.initial_state.authored_fields.map((field) => (
                    <div key={field.name} className="min-w-0">
                      <dt className="text-[var(--nova-text-faint)]">{field.name}</dt>
                      <dd className="min-w-0 whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{typeof field.value === 'string' ? field.value : JSON.stringify(field.value, null, 2)}</dd>
                    </div>
                  )) : <>
                  {draft.initial_state.family ? <Fact label={t('creation.rank')} value={String(draft.initial_state.rank ?? '')} /> : <Fact label={t('creation.realm')} value={draft.initial_state.realm_label ?? draft.initial_state.realm ?? ''} />}
                  <Fact
                    label={draft.initial_state.currency_label || t('creation.currency')}
                    value={
                      draft.initial_state.family ? String(draft.initial_state.currency) : t('creation.currencyValue', { count: draft.initial_state.currency })
                    }
                  />
                  <Fact label={t('creation.items')} value={(draft.initial_state?.items ?? []).join(t('creation.listSeparator'))} />
                  <Fact label={t('creation.equipped')} value={(draft.initial_state?.equipped ?? []).join(t('creation.listSeparator'))} />
                  <Fact label={t(draft.initial_state.family === 'wuxia' ? 'creation.moves' : (draft.initial_state.family === 'space' || draft.initial_state.family === 'fantasy') ? 'creation.skills' : 'creation.techniques')} value={(draft.initial_state.techniques ?? []).join(t('creation.listSeparator'))} />
                  </>}
                </dl>
                <p className="mt-1.5 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t(draft.initial_state.authored_fields?.length ? 'creation.authoredStateHint' : 'creation.stateHint')}</p>
              </section> : null}
            </div>

            {contacts.length > 0 ? (
              <section className="min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2.5">
                <h4 className="text-xs font-medium text-[var(--nova-text-muted)]">{t('creation.contacts')}</h4>
                <ul className="mt-1.5 flex flex-col gap-1 text-xs text-[var(--nova-text-muted)]">
                  {(draft.contacts?.length ? draft.contacts : contacts).map((line) => (
                    <li key={line} className="break-words">{line}</li>
                  ))}
                </ul>
                <p className="mt-1 text-[10px] leading-4 text-[var(--nova-text-faint)]">{t('creation.contactsHint')}</p>
              </section>
            ) : null}

            </fieldset>
            </div>
            <DialogFooter className="mx-0 mb-0 shrink-0 flex-wrap bg-popover">
              <Button
                type="button"
                variant="ghost"
                onClick={() => setStep('choose')}
                disabled={Boolean(busy)}
              >
                <ArrowLeft data-icon="inline-start" />
                {t('creation.modify')}
              </Button>
              <Button
                type="button"
                variant="ghost"
                onClick={() => void requestDraft(true)}
                disabled={Boolean(busy)}
              >
                {busy === 'rewrite' ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <RefreshCw data-icon="inline-start" />}
                {busy === 'rewrite' ? t('creation.rewriting') : t('creation.rewrite')}
              </Button>
              <Button type="button" onClick={() => void start()} disabled={Boolean(busy)}>
                {busy === 'start' ? <Loader2 data-icon="inline-start" className="animate-spin" /> : null}
                {busy === 'start' ? t('creation.starting') : t('creation.start')}
              </Button>
            </DialogFooter>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function OptionChip({ selected, label, onClick }: { selected: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      onClick={onClick}
      className={cn(
        'rounded-full border px-2.5 py-1 text-xs transition-colors',
        selected
          ? 'border-[var(--nova-accent)] bg-[var(--nova-accent)]/10 text-[var(--nova-text)]'
          : 'border-[var(--nova-border)] text-[var(--nova-text-muted)] hover:border-[var(--nova-text-faint)]',
      )}
    >
      {label}
    </button>
  )
}

function OptionSelect({
  label,
  value,
  options,
  onChange,
  hint,
}: {
  label: string
  value: string
  options: StoryPackCreationOption[]
  onChange: (value: string) => void
  hint?: string
}) {
  return (
    <label className="flex min-w-0 flex-col gap-1">
      <span className="text-xs text-[var(--nova-text-muted)]">{label}</span>
      <select aria-label={label} value={value} onChange={(event) => onChange(event.target.value)} className={inputCls}>
        {options.map((option) => (
          <option key={option.id} value={option.id}>{option.name}</option>
        ))}
      </select>
      {hint ? <span className="text-xs leading-5 text-[var(--nova-text-muted)]">{hint}</span> : null}
    </label>
  )
}

function Fact({ label, value }: { label: string; value?: string }) {
  if (!value) return null
  return (
    <div className="flex min-w-0 gap-1.5">
      <dt className="shrink-0 text-[var(--nova-text-faint)]">{label}</dt>
      <dd className="min-w-0 break-words">{value}</dd>
    </div>
  )
}

function rootArity(categoryId: string): number | null {
  switch (categoryId) {
    case 'single':
      return 1
    case 'dual':
      return 2
    case 'triple':
      return 3
    case 'quad':
      return 4
    case 'quinary':
      return 5
    case 'waste':
      return 0
    default:
      // pseudo roots accept one or two, so the count is not enforced here
      return null
  }
}

function defaultOf(options: StoryPackCreationOption[] | undefined): string {
  if (!options || options.length === 0) return ''
  return options.find((option) => option.default)?.id ?? options[0].id
}

function creationDefaults(creation: StoryPackCreation | undefined) {
  const templates = creation?.custom_state_basis_templates?.length ? creation.custom_state_basis_templates : creation?.state_basis_templates ?? []
  // A custom origin starts from the state the pack says it defaults to, not
  // from whichever template happens to be listed first.
  const custom = creation?.origins?.find((origin) => origin.id === 'custom')
  const customBasis = custom?.default_template_id ?? custom?.template_id ?? ''
  return {
    gender: defaultOf(creation?.genders),
    origin: defaultOf(visibleCreationOrigins(creation)),
    stateBasis: templates.includes(customBasis) ? customBasis : templates[0] ?? '',
    rootCategory: defaultOf(creation?.root_categories),
    aptitude: defaultOf(creation?.aptitudes),

    weapon: defaultOf(creation?.weapon_directions),
    adventureClass: defaultOf(creation?.adventure_classes),
    adventureWeapon: defaultOf(creation?.adventure_weapons),
    adventureMethod: defaultOf(creation?.adventure_methods),
  }
}
