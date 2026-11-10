import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, ArrowLeft, Loader2, RefreshCw, Settings2, Sparkles, Wand2 } from 'lucide-react'
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
import { requestSettingsSection } from '@/features/onboarding/events'
import { useStoryModelReadiness } from '@/features/interactive/model-readiness'
import {
  draftStoryPackWorld,
  previewStoryPackWorld,
  saveStoryPackWorld,
  type StoryPackSummary,
  type StoryPackWorldDraft,
  type StoryPackWorldGameplay,
  type StoryPackWorldPreview,
  type StoryPackWorldSeed,
} from '@/lib/api-client'
import { cn } from '@/lib/utils'

interface WorldCreationDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Hands the newly saved world back so the page can select it at once. */
  onSaved: (summary: StoryPackSummary) => void
}

const inputCls =
  'nova-field w-full rounded-[var(--nova-radius)] border px-2.5 py-1.5 outline-none placeholder:text-[var(--nova-text-faint)] focus:border-[var(--nova-field-focus-border)] focus:bg-[var(--nova-surface-3)]'

type Mode = 'guided' | 'automatic'
type Step = 'seed' | 'preview'
type Busy = '' | 'draft' | 'rewrite' | 'save'

function recommendedGameplay(): StoryPackWorldGameplay {
  return {
    status_template: 'world-actions-v1',
    numeric_scale: '1-10',
    growth: 'none',
    combat: 'none',
    communication: 'local',
    letter_delivery_days: 1,
    opening: 'commission',
    state_fields: [
      { name: '观察', description: '发现现场细节与线索的能力。', minimum: 1, maximum: 10, initial: 5 },
      { name: '判断', description: '比较信息并作出判断的能力。', minimum: 1, maximum: 10, initial: 5 },
      { name: '交际', description: '与他人沟通、争取合作的能力。', minimum: 1, maximum: 10, initial: 5 },
      { name: '身手', description: '行动、追逐与脱身的能力。', minimum: 1, maximum: 10, initial: 5 },
    ],
    check_bindings: [
      { action: 'search', field: '观察', scale: 1, offset: -5, minimum: -4, maximum: 5 },
      { action: 'evidence_review', field: '判断', scale: 1, offset: -5, minimum: -4, maximum: 5 },
      { action: 'persuasion', field: '交际', scale: 1, offset: -5, minimum: -4, maximum: 5 },
      { action: 'chase_or_escape', field: '身手', scale: 1, offset: -5, minimum: -4, maximum: 5 },
    ],
    confirmed: false,
  }
}

/**
 * Creating a world: one screen of questions or one sentence, then one preview of
 * the world those answers describe.
 *
 * The two entry points are not two features. They write into the same draft, so
 * switching between them, retrying a failed call or asking for the prose again
 * all keep whatever the player already edited. Nothing is written until 保存世界,
 * and saving is create-only: pressing it twice makes two worlds rather than
 * destroying the first.
 *
 * Everything mechanical — the rules, the state a character starts with, the
 * aptitude tiers — comes back assembled by the server from a supported template.
 * The fields here are prose, and editing them cannot make a world start somebody
 * richer than a built-in one.
 */
export function WorldCreationDialog({ open, onOpenChange, onSaved }: WorldCreationDialogProps) {
  const { t } = useTranslation()
  const modelReady = useStoryModelReadiness()
  const [mode, setMode] = useState<Mode>('guided')
  const [step, setStep] = useState<Step>('seed')
  const [name, setName] = useState('')
  const [genre, setGenre] = useState('')
  const [tone, setTone] = useState('')
  const [protagonist, setProtagonist] = useState('')
  const [openingIdea, setOpeningIdea] = useState('')
  const [brief, setBrief] = useState('')
  const [draft, setDraft] = useState<StoryPackWorldDraft | null>(null)
  const [gameplay, setGameplay] = useState<StoryPackWorldGameplay>(recommendedGameplay)
  const [preview, setPreview] = useState<StoryPackWorldPreview | null>(null)
  const [busy, setBusy] = useState<Busy>('')
  const [error, setError] = useState('')
  // A rewrite regenerates prose from this seed. The current draft still
  // travels with it so author-controlled audiences survive regeneration.
  const seedRef = useRef<StoryPackWorldSeed>({})

  useEffect(() => {
    if (!open) return
    setMode('guided')
    setStep('seed')
    setName('')
    setGenre('')
    setTone('')
    setProtagonist('')
    setOpeningIdea('')
    setBrief('')
    setDraft(null)
    setGameplay(recommendedGameplay())
    setPreview(null)
    setBusy('')
    setError('')
    seedRef.current = {}
  }, [open])

  const seed = (): StoryPackWorldSeed => {
    if (mode === 'automatic') return { mode, brief: brief.trim(), gameplay: { ...gameplay, confirmed: true } }
    return {
      mode,
      name: name.trim() || undefined,
      genre: genre.trim() || undefined,
      brief: brief.trim() || undefined,
      tone: tone.trim() || undefined,
      protagonist: protagonist.trim() || undefined,
      opening_idea: openingIdea.trim() || undefined,
      gameplay: { ...gameplay, confirmed: true },
    }
  }

  const canDraft = mode === 'automatic'
    ? brief.trim().length > 0 && gameplay.status_template !== '' && gameplay.numeric_scale !== '' && gameplay.growth !== '' && gameplay.combat !== '' && gameplay.communication !== '' && gameplay.opening !== ''
    : (brief.trim().length > 0 || Boolean(draft?.origin)) && gameplay.status_template !== '' && gameplay.numeric_scale !== '' && gameplay.growth !== '' && gameplay.combat !== '' && gameplay.communication !== '' && gameplay.opening !== ''

  const requestDraft = async (rewrite: boolean) => {
    if (busy) return
    setBusy(rewrite ? 'rewrite' : 'draft')
    setError('')
    const nextSeed = seed()
    try {
      // The server replaces prose on rewrite while retaining author-controlled
      // audiences. Both paths therefore need the current draft, when one exists.
      const result = await draftStoryPackWorld(nextSeed, draft ?? undefined, rewrite)
      if (result.error || result.error_key) {
        // The draft did not finish. Keep everything the player typed and say
        // why; a failed call must not cost them their work.
        setError(t(result.error_key || result.error || 'worldCreation.draftFailed'))
        return
      }
      setDraft(result.draft)
      setPreview(result.preview)
      seedRef.current = nextSeed
      setStep('preview')
    } catch {
      setError(t('worldCreation.draftFailed'))
    } finally {
      setBusy('')
    }
  }

  const requestRewrite = async () => {
    if (busy || !draft) return
    setBusy('rewrite')
    setError('')
    try {
      const result = await draftStoryPackWorld(seedRef.current, draft, true)
      if (result.error || result.error_key) {
        setError(t(result.error_key || result.error || 'worldCreation.draftFailed'))
        return
      }
      setDraft(result.draft)
      setPreview(result.preview)
    } catch {
      setError(t('worldCreation.draftFailed'))
    } finally {
      setBusy('')
    }
  }

  const save = async () => {
    if (busy || !draft) return
    setBusy('save')
    setError('')
    try {
      const result = await saveStoryPackWorld(draft)
      onSaved(result.summary)
      onOpenChange(false)
    } catch {
      setError(t('worldCreation.saveFailed'))
    } finally {
      setBusy('')
    }
  }

  // Editing a prose field after the draft came back must re-check readiness,
  // otherwise the page could keep offering a start the save would refuse. The
  // check is the save's own three validations with no model call, and it is
  // debounced so typing does not fire one request per keystroke.
  const previewTimer = useRef<number | null>(null)
  const editDraft = (patch: Partial<StoryPackWorldDraft>) => {
    setDraft((current) => {
      if (!current) return current
      const next = { ...current, ...patch }
      if (previewTimer.current !== null) window.clearTimeout(previewTimer.current)
      previewTimer.current = window.setTimeout(() => {
        previewTimer.current = null
        void previewStoryPackWorld(next)
          .then(setPreview)
          .catch(() => { /* the edit is kept; the previous preview stays */ })
      }, 400)
      return next
    })
  }

  useEffect(() => () => {
    if (previewTimer.current !== null) window.clearTimeout(previewTimer.current)
  }, [])

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!busy) onOpenChange(next) }}>
      <DialogContent className="max-h-[90vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{step === 'seed' ? t('worldCreation.title') : t('worldCreation.previewTitle')}</DialogTitle>
          <DialogDescription>
            {step === 'seed' ? t('worldCreation.subtitle') : t('worldCreation.previewSubtitle')}
          </DialogDescription>
        </DialogHeader>

        {error ? (
          <div role="alert" className="flex items-start gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-danger)]/30 bg-[var(--nova-danger)]/5 px-3 py-2 text-xs text-[var(--nova-danger)]">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span className="min-w-0 break-words">{error}</span>
          </div>
        ) : null}

        {step === 'seed' ? (
          <div className="flex min-w-0 flex-col gap-4">
            {/* Drafting a world is the model's work; the answers are not. Saying
                so here, with the way to fix it, keeps a player from writing a
                whole page of setup and then meeting a refusal at the button. */}
            {modelReady === false ? (
              <section
                role="status"
                aria-labelledby="world-creation-model-missing"
                className="rounded-[var(--nova-radius)] border border-[var(--nova-warning)]/30 bg-[var(--nova-warning)]/5 px-3 py-2"
              >
                <h3 id="world-creation-model-missing" className="text-xs font-medium text-[var(--nova-text)]">
                  {t('worldCreation.modelMissingTitle')}
                </h3>
                <p className="mt-0.5 text-[11px] leading-5 text-[var(--nova-text-muted)]">
                  {t('worldCreation.modelMissing')}
                </p>
                <Button
                  type="button"
                  variant="outline"
                  size="xs"
                  className="mt-1.5"
                  onClick={() => requestSettingsSection('model')}
                >
                  <Settings2 data-icon="inline-start" />
                  {t('worldCreation.modelConfigure')}
                </Button>
              </section>
            ) : null}
            <div role="radiogroup" aria-label={t('worldCreation.modeLabel')} className="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
              <ModeCard
                selected={mode === 'guided'}
                onClick={() => setMode('guided')}
                title={t('worldCreation.modeGuided')}
                description={t('worldCreation.modeGuidedHint')}
              />
              <ModeCard
                selected={mode === 'automatic'}
                onClick={() => setMode('automatic')}
                title={t('worldCreation.modeAutomatic')}
                description={t('worldCreation.modeAutomaticHint')}
              />
            </div>

            {mode === 'automatic' ? (
              <label className="flex min-w-0 flex-col gap-1">
                <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.sentence')}</span>
                <Textarea
                  value={brief}
                  rows={3}
                  maxLength={400}
                  placeholder={t('worldCreation.sentencePlaceholder')}
                  onChange={(event) => setBrief(event.target.value)}
                  className={inputCls}
                />
                <span className="text-[10px] leading-4 text-[var(--nova-text-faint)]">
                  {t('worldCreation.sentenceHint')}
                </span>
              </label>
            ) : (
              <div className="flex min-w-0 flex-col gap-3">
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <label className="flex min-w-0 flex-col gap-1">
                    <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.name')}</span>
                    <Input
                      value={name}
                      maxLength={32}
                      placeholder={t('worldCreation.namePlaceholder')}
                      onChange={(event) => setName(event.target.value)}
                      className={inputCls}
                    />
                  </label>
                  <label className="flex min-w-0 flex-col gap-1">
                    <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.genre')}</span>
                    <Input
                      value={genre}
                      maxLength={32}
                      placeholder={t('worldCreation.genrePlaceholder')}
                      onChange={(event) => setGenre(event.target.value)}
                      className={inputCls}
                    />
                  </label>
                </div>
                <label className="flex min-w-0 flex-col gap-1">
                  <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.brief')}</span>
                  <Textarea
                    value={brief}
                    rows={3}
                    maxLength={800}
                    placeholder={t('worldCreation.briefPlaceholder')}
                    onChange={(event) => setBrief(event.target.value)}
                    className={inputCls}
                  />
                </label>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <label className="flex min-w-0 flex-col gap-1">
                    <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.tone')}</span>
                    <Input
                      value={tone}
                      maxLength={32}
                      placeholder={t('worldCreation.tonePlaceholder')}
                      onChange={(event) => setTone(event.target.value)}
                      className={inputCls}
                    />
                  </label>
                  <label className="flex min-w-0 flex-col gap-1">
                    <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.protagonist')}</span>
                    <Input
                      value={protagonist}
                      maxLength={200}
                      placeholder={t('worldCreation.protagonistPlaceholder')}
                      onChange={(event) => setProtagonist(event.target.value)}
                      className={inputCls}
                    />
                  </label>
                </div>
                <label className="flex min-w-0 flex-col gap-1">
                  <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.opening')}</span>
                  <Textarea
                    value={openingIdea}
                    rows={2}
                    maxLength={400}
                    placeholder={t('worldCreation.openingPlaceholder')}
                    onChange={(event) => setOpeningIdea(event.target.value)}
                    className={inputCls}
                  />
                </label>
              </div>
            )}

            <p className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-3)] px-2.5 py-2 text-[11px] leading-5 text-[var(--nova-text-muted)]">
              {t('worldCreation.rulesNote')}
            </p>

            <section className="flex min-w-0 flex-col gap-2 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2.5">
              <h3 className="text-xs font-medium text-[var(--nova-text)]">{t('worldCreation.gameplayTitle')}</h3>
              <p className="text-[11px] leading-5 text-[var(--nova-text-muted)]">{t('worldCreation.gameplayHint')}</p>
              <GameplayAnswer label={t('worldCreation.stateQuestion')} value={gameplay.status_template} options={[["world-actions-v1", t('worldCreation.generalActionState')], ["xiuxian-state", t('worldCreation.cultivationState')]]} onChange={(value) => setGameplay((current) => value === 'xiuxian-state' ? ({ ...current, status_template: value, numeric_scale: 'cultivation', growth: 'cultivation', combat: 'cultivation', communication: 'cultivation-signal' }) : ({ ...current, ...recommendedGameplay(), opening: current.opening }))} className={inputCls} />
              <GameplayAnswer label={t('worldCreation.scaleQuestion')} value={gameplay.numeric_scale} options={gameplay.status_template === 'xiuxian-state' ? [["cultivation", t('worldCreation.cultivationScale')]] : [["1-10", t('worldCreation.scaleOneToTen')]]} onChange={(value) => setGameplay((current) => ({ ...current, numeric_scale: value }))} className={inputCls} />
              <GameplayAnswer label={t('worldCreation.growthQuestion')} value={gameplay.growth} options={gameplay.status_template === 'xiuxian-state' ? [["cultivation", t('worldCreation.cultivationGrowth')]] : [["none", t('worldCreation.noNumericGrowth')]]} onChange={(value) => setGameplay((current) => ({ ...current, growth: value }))} className={inputCls} />
              <GameplayAnswer label={t('worldCreation.combatQuestion')} value={gameplay.combat} options={gameplay.status_template === 'xiuxian-state' ? [["cultivation", t('worldCreation.cultivationCombat')]] : [["none", t('worldCreation.noIndependentCombat')]]} onChange={(value) => setGameplay((current) => ({ ...current, combat: value }))} className={inputCls} />
              {gameplay.status_template === 'world-actions-v1' ? <GameplayAnswer label={t('worldCreation.communicationQuestion')} value={gameplay.communication} options={[["local", t('worldCreation.communicationLocal')], ["letter", t('worldCreation.communicationLetter')]]} onChange={(value) => setGameplay((current) => ({ ...current, communication: value }))} className={inputCls} /> : null}
              {gameplay.status_template === 'world-actions-v1' && gameplay.communication === 'letter' ? <label className="grid min-w-0 grid-cols-1 items-center gap-1 text-xs sm:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]"><span className="font-medium text-[var(--nova-text-muted)]">{t('worldCreation.letterDeliveryDays')}</span><Input type="number" min={1} max={30} value={gameplay.letter_delivery_days ?? 1} onChange={(event) => setGameplay((current) => ({ ...current, letter_delivery_days: Number(event.target.value) }))} className={inputCls} /></label> : null}
              <GameplayAnswer label={t('worldCreation.openingQuestion')} value={gameplay.opening} options={[["commission", t('worldCreation.openingCommission')], ["scene", t('worldCreation.openingAtScene')], ["daily", t('worldCreation.openingDaily')]]} onChange={(value) => setGameplay((current) => ({ ...current, opening: value }))} className={inputCls} />
              {gameplay.status_template === 'world-actions-v1' ? <div className="grid grid-cols-2 gap-2 pt-1 sm:grid-cols-4">
                {(gameplay.state_fields ?? []).map((field, index) => (
                  <label key={index} className="flex min-w-0 flex-col gap-1 text-[11px] text-[var(--nova-text-muted)]">
                    <span>{t('worldCreation.fieldName')}</span>
                    <Input
                      value={field.name}
                      maxLength={16}
                      onChange={(event) => setGameplay((current) => ({
                        ...current,
                        state_fields: (current.state_fields ?? []).map((item, fieldIndex) => fieldIndex === index ? { ...item, name: event.target.value } : item),
                        check_bindings: (current.check_bindings ?? []).map((binding) => binding.field === field.name ? { ...binding, field: event.target.value } : binding),
                      }))}
                      className={inputCls}
                    />
                    <Input
                      type="number"
                      min={field.minimum}
                      max={field.maximum}
                      aria-label={t('worldCreation.fieldInitial', { name: field.name })}
                      value={field.initial}
                      onChange={(event) => setGameplay((current) => ({ ...current, state_fields: (current.state_fields ?? []).map((item, fieldIndex) => fieldIndex === index ? { ...item, initial: Number(event.target.value) } : item) }))}
                      className={inputCls}
                    />
                    <span>{t('worldCreation.fieldRange')}</span>
                    <div className="grid grid-cols-2 gap-1">
                      <Input type="number" min={0} max={19} aria-label={t('worldCreation.fieldMinimum', { name: field.name })} value={field.minimum} onChange={(event) => setGameplay((current) => ({ ...current, state_fields: (current.state_fields ?? []).map((item, fieldIndex) => fieldIndex === index ? { ...item, minimum: Number(event.target.value) } : item) }))} className={inputCls} />
                      <Input type="number" min={1} max={20} aria-label={t('worldCreation.fieldMaximum', { name: field.name })} value={field.maximum} onChange={(event) => setGameplay((current) => ({ ...current, state_fields: (current.state_fields ?? []).map((item, fieldIndex) => fieldIndex === index ? { ...item, maximum: Number(event.target.value) } : item) }))} className={inputCls} />
                    </div>
                  </label>
                ))}
                <div className="col-span-2 flex flex-col gap-2 sm:col-span-4">
                  {(gameplay.check_bindings ?? []).map((binding, index) => (
                    <div key={binding.action} className="grid grid-cols-2 items-center gap-2 text-[11px] text-[var(--nova-text-muted)] sm:grid-cols-4">
                      <span>{t(`worldCreation.action.${binding.action}`)}</span>
                      <select value={binding.field} onChange={(event) => setGameplay((current) => ({ ...current, check_bindings: (current.check_bindings ?? []).map((item, bindingIndex) => bindingIndex === index ? { ...item, field: event.target.value } : item) }))} className={inputCls}>
                        {(gameplay.state_fields ?? []).map((field) => <option key={field.name} value={field.name}>{field.name}</option>)}
                      </select>
                      <label className="flex items-center gap-1">{t('worldCreation.bindingScale')}<Input type="number" min={0} max={2} step={0.1} value={binding.scale} onChange={(event) => setGameplay((current) => ({ ...current, check_bindings: (current.check_bindings ?? []).map((item, bindingIndex) => bindingIndex === index ? { ...item, scale: Number(event.target.value) } : item) }))} className={inputCls} /></label>
                      <label className="flex items-center gap-1">{t('worldCreation.bindingOffset')}<Input type="number" min={-20} max={20} value={binding.offset} onChange={(event) => setGameplay((current) => ({ ...current, check_bindings: (current.check_bindings ?? []).map((item, bindingIndex) => bindingIndex === index ? { ...item, offset: Number(event.target.value) } : item) }))} className={inputCls} /></label>
                    </div>
                  ))}
                </div>
              </div> : null}
            </section>

            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={Boolean(busy)}>
                {t('common.cancel')}
              </Button>
              <Button type="button" onClick={() => void requestDraft(false)} disabled={!canDraft || Boolean(busy)}>
                {busy === 'draft' ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Sparkles data-icon="inline-start" />}
                {busy === 'draft' ? t('worldCreation.drafting') : t('worldCreation.generate')}
              </Button>
            </DialogFooter>
          </div>
        ) : draft ? (
          <div className="flex min-w-0 flex-col gap-4">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label className="flex min-w-0 flex-col gap-1">
                <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.name')}</span>
                <Input
                  value={draft.name}
                  maxLength={32}
                  onChange={(event) => editDraft({ name: event.target.value })}
                  className={inputCls}
                />
              </label>
              <label className="flex min-w-0 flex-col gap-1">
                <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.storyTitle')}</span>
                <Input
                  value={draft.title}
                  maxLength={60}
                  onChange={(event) => editDraft({ title: event.target.value })}
                  className={inputCls}
                />
              </label>
            </div>

            <label className="flex min-w-0 flex-col gap-1">
              <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.description')}</span>
              <Textarea
                value={draft.description ?? ''}
                rows={2}
                maxLength={400}
                onChange={(event) => editDraft({ description: event.target.value })}
                className={inputCls}
              />
            </label>

            <label className="flex min-w-0 flex-col gap-1">
              <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.premise')}</span>
              <Textarea
                value={draft.origin}
                rows={3}
                maxLength={1200}
                onChange={(event) => editDraft({ origin: event.target.value })}
                className={inputCls}
              />
            </label>

            <label className="flex min-w-0 flex-col gap-1">
              <span className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.openingText')}</span>
              <Textarea
                value={draft.opening_text ?? ''}
                rows={3}
                maxLength={1200}
                onChange={(event) => editDraft({ opening_text: event.target.value })}
                className={inputCls}
              />
            </label>

            <section className="min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2.5">
              <h4 className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.content')}</h4>
              <dl className="mt-1.5 flex flex-col gap-1 text-xs">
                <Fact label={t('worldCreation.worldCount')} value={String(preview?.world_count ?? 0)} />
                <Fact label={t('worldCreation.characterCount')} value={String(preview?.character_count ?? 0)} />
                <Fact label={t('worldCreation.origins')} value={(preview?.origins ?? []).join(t('worldCreation.listSeparator'))} />
                <Fact label={t('worldCreation.affiliations')} value={(preview?.affiliations ?? []).join(t('worldCreation.listSeparator'))} />
                <Fact label={t('worldCreation.locations')} value={(preview?.locations ?? []).join(t('worldCreation.listSeparator'))} />
              </dl>
              {(draft.world ?? []).length > 0 ? (
                <ul className="mt-2 flex flex-col gap-1 border-t border-[var(--nova-border)] pt-2 text-xs text-[var(--nova-text-muted)]">
                  {(draft.world ?? []).map((entry) => (
                    <li key={entry.name} className="min-w-0 break-words">
                      <span className="font-medium text-[var(--nova-text)]">{entry.name}</span>
                      {entry.brief_description ? ` · ${entry.brief_description}` : ''}
                    </li>
                  ))}
                </ul>
              ) : null}
              {(draft.characters ?? []).length > 0 ? (
                <ul className="mt-2 flex flex-col gap-1 border-t border-[var(--nova-border)] pt-2 text-xs text-[var(--nova-text-muted)]">
                  {(draft.characters ?? []).map((person) => (
                    <li key={person.name} className="min-w-0 break-words">
                      <span className="font-medium text-[var(--nova-text)]">{person.name}</span>
                      {person.role ? ` — ${person.role}` : ''}
                      {person.brief ? ` · ${person.brief}` : ''}
                    </li>
                  ))}
                </ul>
              ) : null}
            </section>

            <section className="min-w-0 rounded-[var(--nova-radius)] border border-[var(--nova-border)] px-3 py-2.5">
              <h4 className="text-xs font-medium text-[var(--nova-text-muted)]">{t('worldCreation.gameplayTitle')}</h4>
              <dl className="mt-1.5 flex flex-col gap-1 text-xs">
                <Fact label={t('worldCreation.stateQuestion')} value={t(gameplay.status_template === 'xiuxian-state' ? 'worldCreation.cultivationState' : 'worldCreation.generalActionState')} />
                <Fact label={t('worldCreation.scaleQuestion')} value={t(gameplay.status_template === 'xiuxian-state' ? 'worldCreation.cultivationScale' : 'worldCreation.scaleOneToTen')} />
                <Fact label={t('worldCreation.growthQuestion')} value={t(gameplay.status_template === 'xiuxian-state' ? 'worldCreation.cultivationGrowth' : 'worldCreation.noNumericGrowth')} />
                <Fact label={t('worldCreation.combatQuestion')} value={t(gameplay.status_template === 'xiuxian-state' ? 'worldCreation.cultivationCombat' : 'worldCreation.noIndependentCombat')} />
                {gameplay.status_template === 'world-actions-v1' ? <Fact label={t('worldCreation.communicationQuestion')} value={t(gameplay.communication === 'letter' ? 'worldCreation.communicationLetter' : 'worldCreation.communicationLocal')} /> : null}
                {gameplay.status_template === 'world-actions-v1' && gameplay.communication === 'letter' ? <Fact label={t('worldCreation.letterDeliveryDays')} value={t('worldCreation.letterDeliveryDaysValue', { count: gameplay.letter_delivery_days ?? 1 })} /> : null}
                <Fact label={t('worldCreation.openingQuestion')} value={t(`worldCreation.opening${gameplay.opening === 'scene' ? 'AtScene' : gameplay.opening === 'daily' ? 'Daily' : 'Commission'}`)} />
                {gameplay.status_template === 'world-actions-v1' ? <>
                  <Fact label={t('worldCreation.attributes')} value={(gameplay.state_fields ?? []).map((field) => `${field.name} ${field.initial}（${field.minimum}–${field.maximum}）`).join(' · ')} />
                  <Fact label={t('worldCreation.bindings')} value={(gameplay.check_bindings ?? []).map((binding) => `${t(`worldCreation.action.${binding.action}`)} → ${binding.field} ×${binding.scale} ${binding.offset >= 0 ? '+' : ''}${binding.offset}`).join(' · ')} />
                </> : null}
              </dl>
            </section>

            {preview && !preview.startable ? (
              <div role="status" className="rounded-[var(--nova-radius)] border border-[var(--nova-warning)]/30 bg-[var(--nova-warning)]/5 px-3 py-2 text-[11px] leading-5 text-[var(--nova-warning)]">
                <p className="font-medium">{t('worldCreation.notStartable')}</p>
                {(preview.reason_keys ?? []).length > 0 || (preview.reasons ?? []).length > 0 ? (
                  <ul className="mt-1 flex flex-col gap-0.5">
                    {(preview.reason_keys ?? []).map((reasonKey) => (
                      <li key={reasonKey}>{t(reasonKey)}</li>
                    ))}
                    {(preview.reasons ?? []).map((reason) => (
                      <li key={reason} className="break-words">{reason}</li>
                    ))}
                  </ul>
                ) : null}
              </div>
            ) : null}

            <p className="rounded-[var(--nova-radius)] border border-[var(--nova-border)] bg-[var(--nova-surface-3)] px-2.5 py-2 text-[11px] leading-5 text-[var(--nova-text-muted)]">
              {t(preview?.notice || draft.notice || 'worldCreation.rulesNote')}
            </p>

            <DialogFooter className="flex-wrap">
              <Button type="button" variant="ghost" onClick={() => setStep('seed')} disabled={Boolean(busy)}>
                <ArrowLeft data-icon="inline-start" />
                {t('worldCreation.modify')}
              </Button>
              <Button type="button" variant="ghost" onClick={() => void requestRewrite()} disabled={Boolean(busy)}>
                {busy === 'rewrite' ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <RefreshCw data-icon="inline-start" />}
                {busy === 'rewrite' ? t('worldCreation.rewriting') : t('worldCreation.rewrite')}
              </Button>
              <Button type="button" onClick={() => void save()} disabled={Boolean(busy) || !draft.name.trim() || !draft.origin.trim()}>
                {busy === 'save' ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Wand2 data-icon="inline-start" />}
                {busy === 'save' ? t('worldCreation.saving') : t('worldCreation.save')}
              </Button>
            </DialogFooter>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function ModeCard({
  selected,
  onClick,
  title,
  description,
}: {
  selected: boolean
  onClick: () => void
  title: string
  description: string
}) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      onClick={onClick}
      className={cn(
        'min-w-0 rounded-[var(--nova-radius)] border px-2.5 py-2 text-left transition-colors',
        selected
          ? 'border-[var(--nova-accent)] bg-[var(--nova-accent)]/10'
          : 'border-[var(--nova-border)] hover:border-[var(--nova-text-faint)]',
      )}
    >
      <span className="block text-xs font-medium text-[var(--nova-text)]">{title}</span>
      <span className="mt-0.5 block text-[10px] leading-4 text-[var(--nova-text-muted)]">{description}</span>
    </button>
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

function GameplayAnswer({
  label,
  value,
  options,
  onChange,
  className,
}: {
  label: string
  value: string
  options: [string, string][]
  onChange: (value: string) => void
  className: string
}) {
  return (
    <label className="grid min-w-0 grid-cols-1 items-center gap-1 text-xs sm:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
      <span className="font-medium text-[var(--nova-text-muted)]">{label}</span>
      <select value={value} onChange={(event) => onChange(event.target.value)} className={className}>
        {options.map(([optionValue, optionLabel]) => <option key={optionValue} value={optionValue}>{optionLabel}</option>)}
      </select>
    </label>
  )
}
