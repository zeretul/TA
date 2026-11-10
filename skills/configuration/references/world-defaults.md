# World new-game defaults (`world_defaults`)

These are one world's new-game defaults: the planning mode, template, narrative styles, reply length and check settings a game started from this world begins with. They are the "new game defaults" shown on the world page, not a second store.

This resource is bound to one world. `list` and `get` return only the world this conversation belongs to, and `update` writes only that world. No scope is accepted: the binding is the scope.

**These are defaults, not a lock.** Changing them affects only the next game started from this world, never a story already running. To change the game being played, use `story_settings`.

## Operations

| Operation | Meaning |
| --- | --- |
| `list` | The bound world's current defaults, with `revision`. |
| `get` | The same defaults, named by world id. Requires exactly one id, and it must be the bound world. |
| `update` | Change only the fields the player asked for, sending back the `revision` that was read. |

## Fields

Every field may be omitted. Omitted means "leave this value as it is". What is read back is the effective value — the one shown on the world page, with the application fallbacks already filled in, not the blank raw block on disk.

| Field | Type | Meaning |
| --- | --- | --- |
| `revision` | string | The current settings revision. `update` must send back the one it read. |
| `planning_mode` | string | `enabled` or `disabled`. |
| `planning_template_id` | string | The planning template id. The template itself is read and written through the `game_planning` resource. |
| `style_purposes` | object | The narrative style for each of the three purposes; see below. |
| `reply_target_chars` | int | Target characters for one turn. |
| `check_settings` | object | How checks and rule state are handled; see below. |

There is **no** `choice_count` here: how many action suggestions to offer is each story's own choice, not part of the world defaults.

### `style_purposes`

| Field | Type | Meaning |
| --- | --- | --- |
| `main` | string | The narrative style id for main narration. |
| `dialogue` | string | The narrative style id for dialogue. An empty string follows main narration. |
| `combat` | string | The narrative style id for combat. An empty string follows main narration. |

### `check_settings`

| Field | Type | Meaning |
| --- | --- | --- |
| `difficulty_shift` | int | Overall difficulty shift for checks, -2 to 2. |
| `roll_modifier` | int | Overall roll modifier for checks, -20 to 20. |
| `rule_state_consumption_mode` | string | `hybrid_auto` or `suggestions_only`. |
| `rule_visibility_mode` | string | `audit_only` or `public_roll`. |

## Partial writes

These defaults are written to disk as one block, so an update starts from the current record and only the fields named in this request are replaced. Everything else the author wrote in this world — the creative direction above all — survives a partial change and is not wiped by a "just change the length" edit.

## `update` example

```text
config_apply({
  "operation": "update",
  "resource": "world_defaults",
  "id": "custom-ferry-chronicle",
  "revision": "…",
  "value": {
    "reply_target_chars": 2200,
    "check_settings": { "difficulty_shift": 1 }
  }
})
```

## Return

`update` returns a receipt with the resource name, the operation, the world id, the new `revision`, and the complete defaults after the write. Read once more afterwards to confirm the fields the player asked for hold the new values and the rest were left alone, and say clearly that this affects only the next game started.
