import history from './en-US/history'
import worldPresentation from './en-US/worldPresentation'
import game from './en-US/game'
import entryDetails from './en-US/entryDetails'
import speech from './en-US/speech'
import common from './en-US/common'
import agentRuntime from './en-US/agentRuntime'
import remoteAccess from './en-US/remoteAccess'
import chat from './en-US/chat'
import editor from './en-US/editor'
import runtime from './en-US/runtime'
import inlineError from './en-US/inlineError'
import command from './en-US/command'
import home from './en-US/home'
import worldPack from './en-US/worldPack'
import worldCreation from './en-US/worldCreation'
import creation from './en-US/creation'
import router from './en-US/router'
import lore from './en-US/lore'
import loreWorkspace from './en-US/loreWorkspace'
import locale from './en-US/locale'
import layout from './en-US/layout'
import agents from './en-US/agents'
import settingPanel from './en-US/settingPanel'
import storyPicker from './en-US/storyPicker'
import branchTimeline from './en-US/branchTimeline'
import storyStage from './en-US/storyStage'
import snapshot from './en-US/snapshot'
import directorPanel from './en-US/directorPanel'
import settings from './en-US/settings'
import workbench from './en-US/workbench'
import gameSettings from './en-US/gameSettings'
import interactiveLayout from './en-US/interactiveLayout'
import api from './en-US/api'
import changes from './en-US/changes'
import narrativeStyle from './en-US/narrativeStyle'
import agentApproval from './en-US/agentApproval'
import trajectory from './en-US/trajectory'
import characterWindow from './en-US/characterWindow'
import combatWindow from './en-US/combatWindow'
import realmWindow from './en-US/realmWindow'
import questWindow from './en-US/questWindow'
import loreOverview from './en-US/loreOverview'
import groupWindow from './en-US/groupWindow'
import intimacyWindow from './en-US/intimacyWindow'
import chatWindow from './en-US/chatWindow'
import skill from './en-US/skill'
import item from './en-US/item'
import craft from './en-US/craft'
import reason from './en-US/reason'

const enUS = {
  ...history,
  ...common,
  ...entryDetails,
  ...characterWindow,
  ...combatWindow,
  ...realmWindow,
  ...game,
  ...worldPresentation,
  ...questWindow,
  ...loreOverview,
  ...groupWindow,
  ...intimacyWindow,
  ...chatWindow,
  ...skill,
  ...item,
  ...craft,
  ...reason,
  ...speech,
  ...agentRuntime,
  ...remoteAccess,
  ...chat,
  ...editor,
  ...runtime,
  ...inlineError,
  ...command,
  ...home,
  ...worldPack,
  ...worldCreation,
  ...creation,
  ...router,
  ...lore,
  ...loreWorkspace,
  ...locale,
  ...layout,
  ...agents,
  ...settingPanel,
  ...storyPicker,
  ...branchTimeline,
  ...storyStage,
  ...snapshot,
  ...directorPanel,
  ...settings,
  ...workbench,
  ...gameSettings,
  ...interactiveLayout,
  ...api,
  ...changes,
  ...narrativeStyle,
  ...agentApproval,
  ...trajectory,
  'agents.tool.availability.runtimeCheck': 'Runtime check',
} as const

export default enUS
