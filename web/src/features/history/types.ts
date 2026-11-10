export interface HistoryRegion {
  id: string; name: string; heart: number; tax_base: number; disaster: number
  refugees: number; irrigation: number; training: number
}
export interface HistoryMinister {
  name_en?: string; office_en?: string;
  id: string; name: string; office: string; profile: string; domain: string
  capacity: number; knowledge: string[]
}
export type InstructionKind = 'relief' | 'training' | 'construction' | 'pause' | 'add_funds' | 'reassign' | 'resume' | 'inspect' | 'unsupported'
export interface HistoryInstruction {
  id: string; kind: InstructionKind; target?: string; minister_id?: string; budget?: number
  grain?: number; quarters?: number; project_id?: string; not_before_turn?: number
  depends_on?: string[]; reason?: string
}
export interface HistoryPlan { orders?: OrderIntent[]; text: string; source_chat_id?: string; source_advice_id?: string; instructions: HistoryInstruction[] }
export interface PreparedEdict { expected_revision: number; plan: HistoryPlan; outcomes?: HistoryOutcome[] }
export interface HistoryOutcome {
  instruction_id?: string; project_id?: string; status: string; reason: string
  spent?: number; grain_spent?: number; progress?: number
}
export interface HistoryProject {
  id: string; instruction_id: string; edict_id: string; kind: string; target: string
  minister_id: string; budget: number; grain: number; spent: number; grain_spent: number
  reserved: boolean; quarters: number; progress: number; required: number
  not_before_turn: number; depends_on: string[]; status: string; reason: string
}
export interface HistoryChat {
  discarded?: boolean;
  id: string; turn: number; mode: string; minister_ids: string[]; user_message: string
  replies: { minister_id: string; text: string }[]; revision: number
}
export interface HistoryReport {
  world?: WorldProposal;
  discarded?: boolean;
  id: string; turn: number; year: number; quarter: number; opening_treasury: number
  income: number; upkeep: number; upkeep_arrears: number; reserved: number; spent: number
  grain_reserved: number; grain_spent: number; closing_treasury: number
  outcomes: HistoryOutcome[]; events: { region_id: string; severity_delta: number; reason: string }[]
  narrative: string
  applied_adjustments?: { project_id: string; efficiency_pct: number; delay: boolean; reason: string }[]
}
export interface HistoryCheckpoint { id: string; label: string; turn: number; revision: number }
export interface HistoryInspection { revision?: number; discarded?: boolean; id: string; target: string; turn: number; facts: Record<string, unknown> }
export interface HistorySummary {
  archived?: boolean;
  unavailable?: boolean; error_code?: string;
  id: string; name: string; scenario_id: string; revision: number; year: number; quarter: number; turn: number
}
export interface SavedAdvice {id:string;chat_id:string;minister_id:string;text:string;title:string;notes:string;turn:number}
export interface HistoryState extends HistorySummary {
 advice?: SavedAdvice[];
  world?: HistoryWorld; scenario?: HistoryScenarioPack; scenario_digest?: string;
  treasury: number; privy_purse: number; grain: number; regions: HistoryRegion[]
  ministers: HistoryMinister[]; projects: HistoryProject[]
  edicts: { id: string; turn: number; plan: HistoryPlan; outcomes: HistoryOutcome[] }[]
  chats: HistoryChat[]; reports: HistoryReport[]; checkpoints: HistoryCheckpoint[]
  inspections: HistoryInspection[]; last_restore_revision?: number
}
export interface HistoryBootstrap {
  project_id: string; scenarios: { id: string; name: string; description: string }[]; campaigns: HistorySummary[]
}
export interface HistoryReceipt {
  command_id: string; revision: number; kind: string; duplicate: boolean
  state: HistoryState; outcomes: HistoryOutcome[]
}
export type HistoryAction = 'advice' | 'archive' | 'unarchive' | 'direct-edict' | 'chat' | 'prepare-edict' | 'edict' | 'advance' | 'checkpoints' | 'restore' | 'inspect'

export interface LocalText {zh:string;en:string}
export interface WorldEntity {id:string;kind:string;name:LocalText;status:string;location?:string;owner?:string;metrics:Record<string,number>;attributes:Record<string,LocalText>;audience:string[]}
export interface WorldAccount {id:string;name:LocalText;resource:string;owner:string;balance:number;audience:string[]}
export interface WorldEvent {id:string;turn:number;title:string;text:string;entity_ids:string[];order_ids:string[];audience:string[];follow_up?:string;status:string}
export interface WorldOrder {disclosure?:'public'|'sealed';id:string;version:number;edict_id:string;kind:string;text:string;minister_id?:string;targets:string[];condition?:string;depends_on:string[];not_before_turn:number;limits:Record<string,number>;spent:Record<string,number>;status:string;progress:number;last_result:string;created_turn:number}
export interface OrderIntent {disclosure?:'public'|'sealed';action:string;order_id?:string;kind?:string;text?:string;minister_id?:string;targets?:string[];condition?:string;depends_on?:string[];not_before_turn?:number;limits?:Record<string,number>}
export interface WorldTransfer {counterparty_hidden?:boolean;from:string;to:string;resource:string;amount:number;order_id?:string;reason:string}
export interface WorldProposal {new_accounts?:WorldAccount[];transfers:WorldTransfer[];changes:{id:string;metric_deltas:Record<string,number>;reason:string}[];new_entities:WorldEntity[];results:{order_id:string;status:string;progress:number;reason:string;condition_met:boolean}[];events:WorldEvent[]}
export interface HistoryWorld {entities:WorldEntity[];accounts:WorldAccount[];orders:WorldOrder[];events:WorldEvent[];ending?:string;achieved:string[]}
export interface HistoryPresentation {backdrop?:string;portraits?:Record<string,{asset:string;index:number;columns:number;rows:number}>;view_box:[number,number,number,number];regions?:{label?:LocalText;entity_id:string;center:[number,number];rings?:[number,number][][]}[];routes?:[string,string][]}
export interface HistoryScenarioPack {player_entity_id?:string;initial?:HistoryWorld;presentation?:HistoryPresentation;schema:number;id:string;version:string;name:LocalText;description:LocalText;calendar:{name:LocalText;year:number;quarter:number;era_offset:number};metrics:{id:string;name:LocalText;unit:LocalText;min:number;max:number}[];resources:{id:string;name:LocalText;unit:LocalText;max_transaction:number}[];objectives:{id:string;name:LocalText;entity_id:string;metric:string;operator:string;value:number;ending:string}[]}
