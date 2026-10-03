export default {
  customUpdate: {
    title: '更新 sshzy 定制版',
    current: '当前安装版本',
    source: '定制发布源',
    baseline: '官方基线',
    officialNotice: '官方已发布 {version}，尚未整合到这个定制版；不能直接安装官方版。',
    notConfigured: '暂时无法验证定制源。须先完成执行器初始安装、可信公钥和服务器读取凭证配置，才可启用更新。',
    activationDisabled: '部署管理员尚未开启激活。准备产物不会改变当前线上站点。',
    target: '目标定制版本',
    prepare: '准备更新',
    prepareRollback: '准备回滚',
    check: '检查更新',
    activate: '确认更新',
    cancel: '取消准备',
    reload: '加载已验证的新版界面',
    pendingMigrations: '本次有 {count} 项待执行数据库迁移。',
    manualMigration: '这项迁移须走单独维护流程，不能一键激活。',
    switchUpstream: '更新会暂时中断本站。若正在使用本站作为 AI 上游，请先切换上游；激活需要近期双重验证。',
    confirmDowntime: '我授权本次维护窗口、备份和后端/UI 替换。',
    confirmMigrations: '我明确授权执行上方列出的数据库迁移。',
    reconnecting: '服务可能正在切换，正在恢复连接；断连不代表更新成功。',
    failed: '更新未能继续。',
    stages: {
      queued: '准备任务已排队', preflight: '正在校验和准备产物', ready: '准备完成，等待确认',
      draining: '暂停新请求并等待在途任务完成', stopping_backend: '等待后端优雅退出，不强杀进程', backing_up: '后端已停止，正在备份数据库',
      revalidating: '重新核验部署与迁移状态', activating_backend: '正在替换后端',
      verifying_backend: '正在验证后端健康与数据库迁移', activating_ui: '正在切换前端',
      verifying_site: '正在验证完整站点', completed: '更新与验收完成',
      failed: '准备失败，请查看错误', cancelled: '准备已取消',
      rolled_back: '已恢复此前的定制部署', manual_intervention: '需要运维人员恢复维护；未自动还原数据库'
    }
  }
}
