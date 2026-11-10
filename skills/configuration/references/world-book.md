# World book (`world_book`)

## 修仙制作方法

在用户确认方案后，修仙用户世界的 `create` 可附 `craft_method`，原典条目与方法在同一次写入中保存。先读取 `craft_methods`，核对来源和版本；内置世界仍需先另存。方法定义仅影响新开局，旧故事、重开与分支保留冻结版本，不自动授予角色学习资格或库存。

`craft_method` 必须包含 `id`、`name_key`（方法显示名）、`version`、`source`、`art`（alchemy/smithing/talisman/formation）、`minimum_stage`、`minutes_per_batch`、`inputs:[{def_id,quantity}]`、`outputs:[{def_id,quantity,chance}]`、`loss_chance`、`requires_learning:true`、`failure_cost:"consume_inputs"`。同 ID 改动须提高版本，完全相同的重试不增版。`required_tools` 是持有但不消耗的已登记物品 ID；`required_location` 精确匹配正式场景地点，空字符串表示不限地点。不得承诺并未登记的物品效果或填写额外效果字段。

可复用的制符材料为 `material.talisman.paper`、`material.talisman.ink`，真实产物为 `talisman.ward.lower`；炼丹材料 `material.herb.green_vine` 可用于现有 `pill.restore.health.lower`。已有装备 `implement.lower.common` 可作为非消耗工具。材料、成品效果仍由既有物品目录决定。保存后读回方法定义并告知版本；角色须在故事中通过 `skills` 正式学习对应版本后才能用 `craft` 制作。

The world book is one world's authored book: the entries a new game of that world starts with. People, factions, places, items and techniques all live here. It is the same file the world page edits, not a second store — collecting an entry here is the same change the author makes on the world page.

This resource is bound to one world. `list` and `get` return only the world this conversation belongs to, and `create` writes only into that world. No scope is accepted: the binding is the scope.

## Operations

| Operation | Meaning |
| --- | --- |
| `list` | The bound world's book: `world_id`, `writable`, and every entry. |
| `get` | The same book, named by world id. Requires exactly one id, and it must be the bound world. |
| `create` | Add one entry to the bound world's book. |

There is no `update` and no `delete`. Rewriting or removing an authored entry belongs to the world page, where the author sees the whole book. The only write a conversation performs is collecting an entry.

## `writable`

A world that ships with the version has `writable: false`. Its file is not under the data directory, so a write either fails or creates a user file that shadows the shipped world for every game started from it.

Do not attempt `create` when `writable` is `false`. Tell the player this world cannot be written directly and suggest saving it as their own copy first, then collecting into that copy.

## Entry fields

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `audience` | string | no | `public` 为公共资料；`gm` 为作者裁决资料，不自动授予玩家或人物知情。新建省略沿用公共资料语义；更新已有条目时省略保留原值。其他非空值保守按 `gm` 处理。 |
| `entry_id` | string | no | The entry id inside the book. Naming an existing id updates that entry in place. Left out, an id is derived from the name. |
| `name` | string | yes | The name the entry shows under in the book. |
| `type` | string | no | `character`, `faction`, `place`, `item`, `technique`, or another kind the world already uses. Follow the kinds the book already has. |
| `importance` | string | no | `high`, `medium`, `low`, or another value the world already uses. |
| `tags` | string[] | no | Short labels for the entry. |
| `keywords` | string[] | no | Words that should bring this entry to mind. |
| `load_mode` | string | no | `auto`, `resident`, `manual`. Follow the values entries of the same kind already use. |
| `brief_description` | string | no | The one-line description shown beside the name. |
| `content` | string | yes | The entry body: what is durably true about this person, place, faction, item or technique. |
| `source_ref` | string | no | Which story entity this entry came from, written `<story id>/<entity id>`. |

## Identity and idempotence

Whether `create` appends or replaces is decided by two rules:

1. If the book already has the same `source_ref`, it is the same entry even when the name is phrased differently. So retrying a collection updates instead of adding a second entry.
2. If the book already has the same `entry_id`, it is that entry. Use this to land a collection on an entry the author has hand-edited.

A new name that matches an existing entry's name is refused, and the refusal names the entry holding that name. Two entries with one name make every later lookup ambiguous, and a same-named person in another story is a different person. Report the refusal to the player rather than forcing the entry in with a suffix.

## What a collection must not carry

The world book holds durable setting, not this run's current state. Collect the entity's identity and history — who they are, what they have done, what they are known for. Do not write in:

- money that changed hands in this run, debts owed, prices or goods agreed
- an injury, mood or intention that belongs to one moment
- a relationship that exists only in this branch
- anything about the entity that has not been established

A story already in progress keeps reading the range it started with. Collecting only changes the next game started from this world, never a game already running.

## Read before writing

`list` first. Check whether this entity is already in the book under another name or another `source_ref`, and look at how entries of the same kind are typed and tagged. Then `create`.

Read the book again after `create` to confirm the entry is there with the fields that were sent, and report the returned entry id to the player.

## `create` example

```text
config_apply({
  "operation": "create",
  "resource": "world_book",
  "value": {
    "name": "Chen Po",
    "type": "character",
    "importance": "medium",
    "tags": ["ferry household", "sells cakes"],
    "keywords": ["ferry cake", "Aunt Chen"],
    "load_mode": "auto",
    "brief_description": "The old woman who sells ferry cakes at the Qingshi crossing.",
    "content": "Chen Po sells ferry cakes at the Qingshi crossing and bakes one basket a day. She speaks slowly, in a worn voice, and picks no quarrels. She has a son who has poled for the Qinggao guild for ten years.",
    "source_ref": "st_abc/Chen Po"
  }
})
```

## Return

`create` returns a receipt with `pack_id`, `entry_id`, `created`, `content_version`, and the entry as it now stands. When writing onto an existing entry, `created` is `false`. `content_version` moves only when the book actually changed: collecting an entry that is already identical writes nothing and leaves the version where it was.
