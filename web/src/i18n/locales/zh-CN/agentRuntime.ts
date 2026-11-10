const agentRuntime = {
  'agentRuntime.apiModelLabel': "API · {{name}}",
  'agentRuntime.profilesFailed': "无法加载 API 模型档案，请刷新重试。",

  'agentRuntime.modelChangeIdle': "请等待本次执行及待处理问题结束后，再修改模型或推理强度。",
  'agentRuntime.connectionFailed': "连接引擎失败，请检查后重试。",
  'agentRuntime.capabilityUnsupported': "此运行时不支持该操作。",
  'agentRuntime.operationFailed': "执行失败，已确认的内容和变更已保留。",
  'agentRuntime.interrupted': "执行已中断。处理待答问题后可发送新消息继续。",
} as const

export default agentRuntime
