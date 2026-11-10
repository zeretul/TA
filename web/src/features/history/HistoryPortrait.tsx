import { useState } from 'react'
import type { HistoryState } from './types'

export function HistoryPortrait({ state, id, name }: { state: HistoryState; id: string; name: string }) {
  const portrait = state.scenario?.presentation?.portraits?.[id]
  const [failed, setFailed] = useState(false)
  return <span className="history-portrait" aria-hidden="true">
    <span>{name.slice(0, 1)}</span>
    {portrait && !failed && <img src={portrait.asset} alt="" draggable={false} onError={() => setFailed(true)} style={{
      width: `${portrait.columns * 100}%`, height: `${portrait.rows * 100}%`, maxWidth: 'none',
      left: `${-(portrait.index % portrait.columns) * 100}%`, top: `${-Math.floor(portrait.index / portrait.columns) * 100}%`,
    }} />}
  </span>
}
