export default {
  customUpdate: {
    title: 'Update sshzy',
    current: 'Installed version',
    source: 'Custom release source',
    baseline: 'Official baseline',
    officialNotice: 'Official {version} is available but has not been integrated into this custom release.',
    notConfigured: 'The custom source cannot be verified. Initial updater installation, trust keys and server credentials must be configured before updates are enabled.',
    activationDisabled: 'Activation is disabled by the deployment administrator. Preparing a release does not change the live site.',
    target: 'Target custom version',
    prepare: 'Prepare update',
    prepareRollback: 'Prepare rollback',
    check: 'Check updates',
    activate: 'Confirm update',
    cancel: 'Cancel preparation',
    reload: 'Load the verified interface',
    pendingMigrations: '{count} database migrations are pending.',
    manualMigration: 'This migration requires a separate maintenance procedure; one-click activation is blocked.',
    switchUpstream: 'Updates temporarily interrupt this site. Switch your AI upstream before confirming. Recent two-factor verification is required.',
    confirmDowntime: 'I authorize the maintenance window, backup, and backend/UI replacement.',
    confirmMigrations: 'I authorize exactly the database migrations listed above.',
    reconnecting: 'The service may be switching. Reconnecting does not mean the update has succeeded.',
    failed: 'The update could not proceed.',
    stages: {
      queued: 'Preparation queued', preflight: 'Verifying and preparing artifacts', ready: 'Prepared; awaiting your confirmation',
      draining: 'Pausing new requests and draining active tasks', stopping_backend: 'Waiting for graceful backend shutdown', backing_up: 'Backing up while the backend is stopped',
      revalidating: 'Rechecking deployment and migration state', activating_backend: 'Replacing the backend',
      verifying_backend: 'Verifying backend health and migrations', activating_ui: 'Switching the interface',
      verifying_site: 'Verifying the complete site', completed: 'Update verified and completed',
      failed: 'Preparation failed; inspect the error', cancelled: 'Preparation cancelled',
      rolled_back: 'Previous custom deployment restored', manual_intervention: 'Maintenance requires operator recovery; no automatic database restore was performed'
    }
  }
}
