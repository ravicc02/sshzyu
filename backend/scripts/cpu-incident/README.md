# CPU 事件本地复现工具

仅用于本机 Docker 的独立测试容器。不要把脚本指向生产数据库。此目录不包含自动执行的数据库迁移。

## 数据与查询计划

在仓库根目录运行以下 PowerShell 命令。容器名、端口和数据库名固定为本次测试专用值；数据库已存在时不要重复造数。

```powershell
docker run -d --name sshzy-cpu-bench-20261104 -p 127.0.0.1:55438:5432 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=cpu_bench postgres:18-alpine
Get-Content -Raw backend/scripts/cpu-incident/seed.sql | docker exec -i sshzy-cpu-bench-20261104 psql -U postgres -d cpu_bench -v ON_ERROR_STOP=1
Get-Content -Raw backend/scripts/cpu-incident/rollup-schema.sql | docker exec -i sshzy-cpu-bench-20261104 psql -U postgres -d cpu_bench -v ON_ERROR_STOP=1
python backend/scripts/cpu-incident/measure.py --output records/cpu-incident-20261104/reproduction
$env:CPU_BENCH_DSN = 'postgres://postgres@127.0.0.1:55438/cpu_bench?sslmode=disable'
go -C backend run ./cmd/cpu-bench
```

造数脚本生成 1,000,000 行 usage_logs 和 31 个分组。表仅保留被测 SQL 使用的字段，并建立时间索引和分组时间索引；它不是生产完整 schema，也不模拟生产的 27 个索引和行宽。measure.py 从原始 r7 提交与当前源码提取真实 ListGroups 查询，分别预热后保存 EXPLAIN (ANALYZE, BUFFERS)。

分组元数据查询代表近期模型缓存命中路径。冷刷新还会执行 account_scheduler_recent_models.go 中的聚合 SQL；其开销必须单独计入，不能把缓存命中的计划当作所有请求的延迟。

Go 入口只接受 localhost 或 127.0.0.1 上名为 cpu_bench 的数据库，调用真实仓储实现 3 次，输出扫描行数、水位和桶数。closed_before 作为已有的持久 last_processed_at，精度为服务时区自然日；本次不增加 schema 字段。每次只处理一天，历史积压需要后续调度逐日追赶。

## CPU 基线

使用同一 r7 来源的修改前和修改后 Linux 二进制、相同依赖容器和相同 2 核 quota。服务启动并稳定后，运行：

```powershell
python backend/scripts/cpu-incident/sample-idle.py sshzy-cpu-before-20261104 --output records/cpu-incident-20261104/idle-cpu-before.json
python backend/scripts/cpu-incident/sample-idle.py sshzy-cpu-after-20261104 --output records/cpu-incident-20261104/idle-cpu-after.json
```

脚本读取 /proc/1/stat 的 utime、stime，按 getconf CLK_TCK 换算实际进程 CPU 秒，每 15 秒采样一次，共 60 秒。core_percent 按一个 CPU 核心计，不是整机百分比。采样期间不发送网关业务请求。

本次 CPU 基线使用独立 cpu_app 空业务库；百万行 SQL 计划使用 cpu_bench。二者不能混写成生产量级端到端性能实测。

## 索引审阅

audit-indexes.sql 只读，输出 idx_scan、物理大小、完整定义、统计重置时间和满足严格前缀条件的候选。生产数据须由维护者另行授权采集。index-candidates.review.sql 中所有 DROP 与 CREATE 都是注释，只有逐项审阅后才能由维护者决定执行。

effective_* 是模型表达式优先索引；created_* 是时间优先索引。二者既不是相同表达式，也不是同一列序，不构成相互覆盖的删除证据。主键、唯一约束、不同谓词、不同 opclass/collation 和包含列均须排除。

## 验证命令

```powershell
go -C backend test ./internal/service/...
go -C backend test -tags unit ./internal/repository -run '^Test(CPU|AccountSchedulerRecent|DashboardAggregationRepositorySyncGroupUsage)' -count=1
go -C backend test ./pkg/deployment ./internal/pkg/timezone -count=1
```

本地无法复现生产宿主机 steal；不根据这些数据推算生产 CPU 降幅。
