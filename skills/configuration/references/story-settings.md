# Story game settings (`story_settings`)

These are one story's own play settings: planning mode and template, which narrative style main narration, dialogue and combat each use, reply length, how many action suggestions to offer, and the check settings. They are the record the game settings page edits, not a second store.

This resource is bound to one story. `list` and `get` return only the story this conversation belongs to, and `update` writes only that story. No scope is accepted: the binding is the scope. Other stories and this world's new-game defaults are separate records; changing these touches none of them.

## Operations

| Operation | Meaning |
| --- | --- |
| `list` | The bound story's current settings, with `revision`. |
| `get` | The same settings, named by story id. Requires exactly one id, and it must be the bound story. |
| `update` | Change only the fields the player asked for, sending back the `revision` that was read. |

There is no `create` and no `delete`: settings exist with the story, and there is no "add another one".

## Fields

Every field may be omitted. Omitted means "leave this value as it is", so changing only the reply length does not also clear the three narrative styles. What is read back is complete; sending the read values back unchanged is still one complete write.

| Field | Type | Meaning |
| --- | --- | --- |
| `revision` | string | The current content revision of these settings. `update` must send back the one it read. |
| `planning_mode` | string | `enabled` or `disabled`. When disabled, this story no longer maintains a mutable branch plan. |
| `planning_template_id` | string | The planning template id. The template itself is read and written through the `game_planning` resource. |
| `style_purposes` | object | Which narrative style each of the three purposes uses; see below. |
| `reply_target_chars` | int | Target characters for one turn. Defaults to 2000. |
| `choice_count` | int | Action suggestions offered per turn, 2–10, default 5. |
| `check_settings` | object | How checks and rule state are handled; see below. |

### `style_purposes`

| Field | Type | Meaning |
| --- | --- | --- |
| `main` | string | The narrative style id for main narration. |
| `dialogue` | string | The narrative style id for dialogue. An empty string follows main narration again. |
| `combat` | string | The narrative style id for combat. An empty string follows main narration again. |

The three are independent: changing only `dialogue` leaves the other two alone. Available style ids come from the `narrative_style` resource; never invent an id.

### `check_settings`

| Field | Type | Meaning |
| --- | --- | --- |
| `difficulty_shift` | int | Overall difficulty shift for checks, -2 to 2. |
| `roll_modifier` | int | Overall roll modifier for checks, -20 to 20. |
| `rule_state_consumption_mode` | string | `hybrid_auto` or `suggestions_only`. The former has the runtime deduct rule state from check outcomes automatically; the latter only suggests and lets the player confirm. |
| `rule_visibility_mode` | string | `audit_only` or `public_roll`. The former keeps the check process in the audit only; the latter shows the roll to the player. |

These tune only this story's check feel; they do not change the rule system itself. The rule system is read and written through the `rule_system` resource.

## Concurrency

`revision` is a content revision, not a timestamp. Advancing a turn changes the story's update time but does not change these fields, so a timestamp would misread the player's own requested change as a conflict. When the page changed settings in the meantime, `update` is refused and reports both revisions — read again and re-apply the player's change; do not blindly retry the same revision.

## `update` example

```text
config_apply({
  "operation": "update",
  "resource": "story_settings",
  "id": "st_abc",
  "revision": "settings-1a2b3c4d5e6f7788",
  "value": {
    "reply_target_chars": 2600,
    "style_purposes": { "combat": "terse-battle" }
  }
})
```

That call changed only the reply length and the combat style; main narration, dialogue, suggestion count and check settings stay as they were.

## Return

`update` returns a receipt with the resource name, the operation, the story id, the new `revision`, and the complete settings after the write. Read once more afterwards to confirm the fields the player asked for hold the new values and the rest were left alone, then tell the player what changed.
