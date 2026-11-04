"""Sample PID 1 CPU ticks, excluding docker-exec helper process CPU.
Example: python backend/scripts/cpu-incident/sample-idle.py sshzy-cpu-after-20261104 --output after.json
"""
import argparse,json,pathlib,subprocess,time
def docker(*args):return subprocess.check_output(['docker',*args],text=True,encoding="utf-8",timeout=10).strip()
def read(container):
 stat=docker('exec',container,'cat','/proc/1/stat'); fields=stat[stat.rfind(')')+2:].split()
 return {'at':time.time(),'ticks':int(fields[11])+int(fields[12])}
def main():
 parser=argparse.ArgumentParser();parser.add_argument('container',choices=['sshzy-cpu-before-20261104','sshzy-cpu-after-20261104']);parser.add_argument('--output',type=pathlib.Path,required=True);args=parser.parse_args()
 hz=int(docker('exec',args.container,'getconf','CLK_TCK'));start=read(args.container);samples=[]
 for _ in range(4):time.sleep(15);samples.append(read(args.container))
 elapsed=samples[-1]['at']-start['at'];cpu=(samples[-1]['ticks']-start['ticks'])/hz
 result={'start':start,'samples':samples,'elapsed_s':elapsed,'cpu_s':cpu,'core_percent':cpu/elapsed*100,'clock_ticks':hz}
 args.output.write_text(json.dumps(result,indent=2),encoding='utf-8');print(json.dumps(result,indent=2))
if __name__=='__main__':main()
