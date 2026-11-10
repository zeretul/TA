/**
 * A handoff from another page into the one assistant.
 *
 * The lore library used to open a second assistant workbench beside the editor.
 * It now hands the entry it is showing to the assistant in Game settings, so
 * this describes what that assistant should open with rather than a surface of
 * its own.
 */
export interface AssistantHandoff {
  /** Distinct per request; one instruction is never sent twice. */
  key: string
  /** Sent on arrival. Empty when the entry only hands over its context. */
  instruction: string
  /** Lore entry the player was editing, or '' when they came from the page. */
  resourceId: string
  /**
   * Story the handoff was made from. The assistant writes into that story's
   * world, so a handoff that names one keeps the write there even when the
   * project's selection moves before the turn runs. Absent means "whatever
   * story the settings page is showing".
   */
  storyId?: string
  /** Page context the assistant needs, e.g. the active lore name and item count. */
  context: Record<string, string>
}
