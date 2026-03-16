import { execSync, execFileSync } from 'child_process'
import { existsSync } from 'fs'

const E2E_DB_PATH = '/tmp/arca-e2e.db'

/**
 * Playwright global setup that cleans up leftover LXD containers from the
 * previous E2E run. Only deletes containers whose IDs are found in the old
 * E2E database, so parallel runs (using separate DB files) are not affected.
 */
export default function globalSetup() {
  cleanupLxdContainersFromDB()
}

function lxdContainerName(machineId: string): string {
  return `arca-machine-${machineId.slice(0, 12)}`
}

function cleanupLxdContainersFromDB() {
  if (!existsSync(E2E_DB_PATH)) {
    return
  }

  let rows: string
  try {
    rows = execFileSync('sqlite3', [E2E_DB_PATH, 'SELECT id FROM machines;'], {
      encoding: 'utf-8',
      timeout: 10_000,
    }).trim()
  } catch {
    return
  }

  if (rows === '') {
    return
  }

  const containerNames = rows.split('\n').map((id) => lxdContainerName(id.trim()))

  console.log(`[global-setup] Cleaning up ${containerNames.length} LXD container(s) from previous run`)

  for (const name of containerNames) {
    try {
      execSync(`sudo lxc delete --force ${name}`, {
        encoding: 'utf-8',
        timeout: 60_000,
      })
      console.log(`[global-setup]   deleted ${name}`)
    } catch {
      // Container may already be gone — ignore
    }
  }
}
