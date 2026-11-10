const agentRuntime = {
  'agentRuntime.apiModelLabel': "API · {{name}}",
  'agentRuntime.profilesFailed': "Could not load API model profiles. Refresh and retry.",

  'agentRuntime.modelChangeIdle': "Wait for this execution and any pending questions to finish before changing the model or effort.",
  'agentRuntime.connectionFailed': "Could not connect to the engine. Check the connection and retry.",
  'agentRuntime.capabilityUnsupported': "This runtime does not support this operation.",
  'agentRuntime.operationFailed': "Execution failed. Confirmed content and changes have been retained.",
  'agentRuntime.interrupted': "Execution was interrupted. Resolve pending questions, then send a new message to continue.",
} as const

export default agentRuntime
