/**
 * The journey entry's own shapes.
 *
 * They live in their own file for the same reason the rest entry's do: nothing
 * here is a second copy of a server shape. Every field is one the journey routes
 * actually return, and every duration is the route rules' own number.
 */

/** One registered connection the entry can offer. */
export interface JourneyRoute {
  route_id: string
  route_version: string
  /** Optional authored name distinguishes discovered alternatives to one place. */
  name?: string
  name_key?: string
  origin_name_key?: string
  destination_name_key?: string
  origin_location_id: string
  /** The origin in the story's own wording. A label, never parsed. */
  origin_label: string
  destination_location_id: string
  destination_label: string
  /** The authored walking time for this connection, in in-fiction minutes. */
  total_minutes: number
  movement_method: string
}

/** A road already under way, so it can be continued rather than restarted. */
export interface JourneyProgress {
  journey_id: string
  route_id: string
  origin: string
  destination: string
  /** Distance already walked, counted from the origin. */
  elapsed_minutes: number
  total_minutes: number
  remaining_minutes: number
  /**
   * True when the character is between two places rather than at one. The entry
   * then names no location, because naming the last stop would claim a place
   * they have already left.
   */
  mid_segment: boolean
}

/**
 * The journey entry's read: where the character stands, the connections that
 * start there, and any road already under way.
 *
 * Reading it costs no world time, so opening the entry never moves anything.
 */
export interface JourneyState {
  story_id: string
  branch_id: string
  clock_seconds: number
  /** The story's own display text for the origin. A label, never parsed. */
  anchor?: string
  /** The scene's committed location text, which is where the character stands. */
  location?: string
  location_id?: string
  routes: JourneyRoute[]
  active?: JourneyProgress
}

/** One committed stretch of travel, as the write reports it. */
export interface JourneyResult {
  story_id: string
  branch_id: string
  event_id: string
  journey_id: string
  route_id: string
  route_version: string
  origin: string
  destination: string
  /** The distance this stretch really covered. Zero when the answer is a replay. */
  added_minutes: number
  /** The same distance in natural wording, such as 一时辰. */
  elapsed?: string
  /** Distance walked so far and the road's length, both the rules' own numbers. */
  elapsed_minutes: number
  total_minutes: number
  remaining_minutes: number
  arrived: boolean
  /** Where the character now stands, empty when the stop is mid-segment. */
  location?: string
  mid_segment: boolean
  clock_seconds: number
  /** True when this answer came from a stretch already committed under the same command. */
  replayed: boolean
}
