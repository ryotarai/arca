import { execFileSync } from 'node:child_process'

const e2eDBPath = '/tmp/arca-e2e.db'

export function directSqlite3(query: string) {
  for (let attempt = 0; attempt < 6; attempt += 1) {
    try {
      execFileSync('sqlite3', [e2eDBPath, `PRAGMA busy_timeout = 5000;\n${query}`], {
        stdio: 'pipe',
      })
      return
    } catch (error) {
      if (!String(error).includes('database is locked') || attempt === 5) {
        throw error
      }
    }
  }
}

export function updateMachineForRestartVisibility(machineID: string, status: string) {
  directSqlite3(
    `UPDATE machines SET setup_version = 'legacy-version' WHERE id = '${machineID}';\nUPDATE machine_states SET status = '${status}' WHERE machine_id = '${machineID}';`,
  )
}
