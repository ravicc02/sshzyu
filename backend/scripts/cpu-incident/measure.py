"""Local-only SQL reproduction. Requires Docker and an initialized cpu_bench.

Start: docker run -d --name sshzy-cpu-bench-20261104 -p 127.0.0.1:55438:5432 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=cpu_bench postgres:18-alpine
Seed: Get-Content -Raw backend/scripts/cpu-incident/seed.sql | docker exec -i sshzy-cpu-bench-20261104 psql -U postgres -d cpu_bench -v ON_ERROR_STOP=1
Usage: python backend/scripts/cpu-incident/measure.py --output <existing-directory>
"""
import argparse,json,pathlib,re,subprocess
ROOT=pathlib.Path(__file__).resolve().parents[3]
def sql(query):
 result=subprocess.run(['docker','exec','-i','sshzy-cpu-bench-20261104','psql','-X','-q','-t','-A','-U','postgres','-d','cpu_bench','-v','ON_ERROR_STOP=1'],input=query,text=True,encoding="utf-8",capture_output=True,check=True,timeout=60)
 return result.stdout.strip()
def extract(source):
 return re.search(r"ListGroups[\s\S]*?QueryContext\(ctx, `([\s\S]*?)`, service.StatusActive",source).group(1).replace('$1',"'active'")
def main():
 parser=argparse.ArgumentParser();parser.add_argument('--output',type=pathlib.Path,required=True);args=parser.parse_args();args.output.mkdir(parents=True,exist_ok=True)
 before=subprocess.check_output(['git','show','97e151ac2:backend/internal/repository/account_scheduler_repo.go'],cwd=ROOT,text=True,encoding="utf-8")
 after=(ROOT/'backend/internal/repository/account_scheduler_repo.go').read_text(encoding='utf-8')
 count=int(sql('SELECT count(*) FROM usage_logs'));assert count>=1000000,count
 results={'usage_rows':count}
 for name,query in [('before',extract(before)),('after',extract(after))]:
  sql('EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) '+query)
  plan=json.loads(sql('EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) '+query));(args.output/(name+'-groups-explain.json')).write_text(json.dumps(plan,indent=2),encoding='utf-8')
  results[name]={'execution_ms':plan[0]['Execution Time'],'shared_buffers':sum(plan[0]['Plan'].get(k,0) for k in ['Shared Hit Blocks','Shared Read Blocks'])}
 cold_source=(ROOT/'backend/internal/repository/account_scheduler_recent_models.go').read_text(encoding='utf-8')
 cold=re.search(r"QueryContext\(queryCtx, `([\s\S]*?)`\)",cold_source).group(1)
 bounds=json.loads(sql("SELECT json_build_object('start',date_trunc('day',min(created_at) AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai','end',(date_trunc('day',min(created_at) AT TIME ZONE 'Asia/Shanghai')+interval '1 day') AT TIME ZONE 'Asia/Shanghai','today',date_trunc('day',now() AT TIME ZONE 'Asia/Shanghai') AT TIME ZONE 'Asia/Shanghai') FROM usage_logs"))
 old_rollup=subprocess.check_output(['git','show','97e151ac2:backend/internal/repository/custom_group_usage_rollup_repo.go'],cwd=ROOT,text=True,encoding="utf-8")
 old_rollup=re.search(r"ExecContext\(ctx, `\s*(INSERT INTO usage_group_daily_rollups[\s\S]*?)`, rebuildStart.UTC",old_rollup).group(1)
 new_rollup=(ROOT/'backend/internal/repository/custom_group_usage_rollup_repo.go').read_text(encoding='utf-8')
 new_rollup=re.search(r"scanSingleRow\(ctx, r.sql, `\s*(WITH source AS MATERIALIZED[\s\S]*?)`, \[\]any",new_rollup).group(1)
 def substitute(query,end):return query.replace('$1',"TIMESTAMPTZ '"+bounds['start']+"'").replace('$2',"TIMESTAMPTZ '"+end+"'").replace('$3',"'Asia/Shanghai'")
 for name,query,mutates in [('cold-recent-models',cold,False),('before-rollup',substitute(old_rollup,bounds['today']),True),('after-rollup',substitute(new_rollup,bounds['end']),True)]:
  command='EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) '+query
  if mutates:command='BEGIN; '+command+'; ROLLBACK;'
  plan=json.loads(sql(command));(args.output/(name+'-explain.json')).write_text(json.dumps(plan,indent=2),encoding='utf-8')
  results[name]={'execution_ms':plan[0]['Execution Time'],'shared_buffers':sum(plan[0]['Plan'].get(k,0) for k in ['Shared Hit Blocks','Shared Read Blocks'])}
 print(json.dumps(results,indent=2))
if __name__=='__main__': main()
