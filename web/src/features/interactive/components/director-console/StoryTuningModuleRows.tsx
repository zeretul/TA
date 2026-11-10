import { TuningRow, TuningSelect, type TuningSelectGroup, type TuningSelectOption, type TuningSource } from './StoryTuningControls'

interface ModuleSelectRowProps {
  label: string
  value: string
  moduleDisabled: boolean
  options: TuningSelectOption[]
  /** Present when the list carries a suggestion that has to read as one. */
  groups?: TuningSelectGroup[]
  busy: boolean
  disabled: boolean
  locked?: boolean
  onChange: (value: string) => void
}

export function ModuleSelectRow({ label, value, moduleDisabled, options, groups, busy, disabled, locked, onChange }: ModuleSelectRowProps) {
  const visibleOptions = includeIDs(options, value)
  return (
    <TuningRow title={label} source={moduleSelectSource(locked, moduleDisabled)} busy={busy} disabled={disabled}>
      <TuningSelect
        value={value || visibleOptions[0]?.id || ''}
        options={visibleOptions}
        groups={groups}
        label={label}
        disabled={disabled}
        onChange={onChange}
      />
    </TuningRow>
  )
}

function moduleSelectSource(locked: boolean | undefined, disabled: boolean): TuningSource {
  if (locked) return 'locked'
  if (disabled) return 'off'
  return 'story'
}

function includeIDs(options: TuningSelectOption[], ...ids: string[]): TuningSelectOption[] {
  const result = [...options]
  for (const id of ids) {
    if (id && !result.some((option) => option.id === id)) result.push({ id, label: id })
  }
  return result
}
