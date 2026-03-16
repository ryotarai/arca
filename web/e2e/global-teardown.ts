import { execSync, execFileSync } from 'child_process'
import { existsSync } from 'fs'

const E2E_DB_PATH = '/tmp/arca-e2e.db'

/**
 * Playwright global teardown that force-deletes LXD containers created during
 * this E2E run. Ensures containers are cleaned up even if individual test
 * cleanup was skipped due to failures or timeouts.
 */
export default function globalTeardown() {
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

  console.log(`[global-teardown] Cleaning up ${containerNames.length} LXD container(s)`)

  for (const name of containerNames) {
    try {
      execSync(`sudo lxc delete --force ${name}`, {
        encoding: 'utf-8',
        timeout: 60_000,
      })
      console.log(`[global-teardown]   deleted ${name}`)
    } catch {
      // Container may already be gone — ignore
    }
  }
}
