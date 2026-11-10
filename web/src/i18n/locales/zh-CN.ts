import history from './zh-CN/history'
import worldPresentation from './zh-CN/worldPresentation'
import game from './zh-CN/game'
import entryDetails from './zh-CN/entryDetails'
import speech from './zh-CN/speech'
import common from './zh-CN/common'
import agentRuntime from './zh-CN/agentRuntime'
import remoteAccess from './zh-CN/remoteAccess'
import chat from './zh-CN/chat'
import editor from './zh-CN/editor'
import runtime from './zh-CN/runtime'
import inlineError from './zh-CN/inlineError'
import command from './zh-CN/command'
import home from './zh-CN/home'
import worldPack from './zh-CN/worldPack'
import worldCreation from './zh-CN/worldCreation'
import creation from './zh-CN/creation'
import router from './zh-CN/router'
import lore from './zh-CN/lore'
import loreWorkspace from './zh-CN/loreWorkspace'
import locale from './zh-CN/locale'
import layout from './zh-CN/layout'
import agents from './zh-CN/agents'
import settingPanel from './zh-CN/settingPanel'
import storyPicker from './zh-CN/storyPicker'
import branchTimeline from './zh-CN/branchTimeline'
import storyStage from './zh-CN/storyStage'
import snapshot from './zh-CN/snapshot'
import directorPanel from './zh-CN/directorPanel'
import settings from './zh-CN/settings'
import workbench from './zh-CN/workbench'
import gameSettings from './zh-CN/gameSettings'
import interactiveLayout from './zh-CN/interactiveLayout'
import api from './zh-CN/api'
import changes from './zh-CN/changes'
import narrativeStyle from './zh-CN/narrativeStyle'
import agentApproval from './zh-CN/agentApproval'
import trajectory from './zh-CN/trajectory'
import characterWindow from './zh-CN/characterWindow'
import combatWindow from './zh-CN/combatWindow'
import realmWindow from './zh-CN/realmWindow'
import questWindow from './zh-CN/questWindow'
import loreOverview from './zh-CN/loreOverview'
import groupWindow from './zh-CN/groupWindow'
import intimacyWindow from './zh-CN/intimacyWindow'
import chatWindow from './zh-CN/chatWindow'
import skill from './zh-CN/skill'
import item from './zh-CN/item'
import craft from './zh-CN/craft'
import reason from './zh-CN/reason'

const zhCN = {
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
  'agents.tool.availability.runtimeCheck': '运行时检查',
} as const

export default zhCN
