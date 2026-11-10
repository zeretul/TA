/**
 * Where one-off verification scripts write their output.
 *
 * Development caches belong under the development root's `.ta-dev`, next to the
 * build, module, npm and playwright caches the repo already keeps there. These
 * scripts used to default to a `denova-cache` folder on whichever drive happened
 * to have room, which scattered run output across the machine and left the next
 * person guessing where a result went.
 *
 * A git worktree reports its own root first, but its caches are the main
 * checkout's — one `.ta-dev` per repository, not one per worktree.
 */
import { existsSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'

function isDirectory(p) {
  try {
    return statSync(p).isDirectory()
  } catch {
    return false
  }
}

/** The directory holding the real `.git`, following a worktree's gitdir link. */
function mainCheckoutRoot(repoRoot) {
  const dotGit = join(repoRoot, '.git')
  if (isDirectory(dotGit)) return repoRoot
  if (!existsSync(dotGit)) return repoRoot
  // A worktree's .git is a file: "gitdir: <main>/.git/worktrees/<name>".
  const match = /^gitdir:\s*(.+)$/m.exec(readFileSync(dotGit, 'utf8'))
  if (!match) return repoRoot
  const gitDir = resolve(repoRoot, match[1].trim())
  // <main>/.git/worktrees/<name> -> <main>
  const candidate = dirname(dirname(dirname(gitDir)))
  return existsSync(join(candidate, 'go.mod')) || existsSync(join(candidate, '.git'))
    ? candidate
    : repoRoot
}

/** The development root: the nearest directory holding go.mod or .git. */
export function devRoot(from = process.cwd()) {
  let dir = resolve(from)
  for (;;) {
    if (existsSync(join(dir, 'go.mod')) || existsSync(join(dir, '.git'))) {
      return mainCheckoutRoot(dir)
    }
    const parent = dirname(dir)
    if (parent === dir) return resolve(from)
    dir = parent
  }
}

// TA_DEV_TOOLS moves the caches, temp files and toolchain off the checkout
// onto a dedicated drive; TA_DEV_OUT names the output root directly.
export const DEV_OUT = process.env.TA_DEV_OUT
  ? resolve(process.env.TA_DEV_OUT)
  : process.env.TA_DEV_TOOLS
    ? join(resolve(process.env.TA_DEV_TOOLS), 'runs')
    : join(devRoot(), '.ta-dev')

/** A path under `.ta-dev`, given with forward slashes for readability. */
export function devOutput(relative) {
  return join(DEV_OUT, ...relative.split('/').filter(Boolean))
}
