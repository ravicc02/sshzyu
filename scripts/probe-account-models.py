#!/usr/bin/env python3
# 只读探测：逐个账号、逐个 model_mapping 里的上游模型名做真实调用，判断是否可调。
# 凭证只从数据库读取，不打印任何 key。
import json
import subprocess
import sys
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor

SQL = """
select coalesce(json_agg(t),'[]') from (
  select id, platform,
         credentials->>'base_url' as base_url,
         credentials->>'api_key'  as api_key,
         credentials->'model_mapping' as mm
  from accounts
  where id >= 296 and deleted_at is null
  order by id
) t;
"""


def load_accounts():
    p = subprocess.run(
        ["docker", "exec", "-i", "sub2api-postgres", "sh", "-c",
         'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tA -f -'],
        input=SQL, capture_output=True, text=True)
    if p.returncode != 0:
        print("PSQL_ERROR:", p.stderr[:300], file=sys.stderr)
        sys.exit(1)
    raw = p.stdout.strip()
    return json.loads(raw) if raw else []


def call(base_url, api_key, platform, model):
    """用 curl 调用：部分上游（Cloudflare）会按 UA 拦截 Python urllib，curl 是实测可用路径。"""
    url = base_url.rstrip("/")
    body = {"model": model, "max_tokens": 8,
            "messages": [{"role": "user", "content": "hi"}]}
    cmd = ["curl", "-s", "-m", "100", "-o", "-", "-w", "\n%{http_code}",
           "-X", "POST", url + ("/v1/messages" if platform == "anthropic"
                                else "/v1/chat/completions"),
           "-H", "Content-Type: application/json",
           "-d", json.dumps(body)]
    if platform == "anthropic":
        cmd += ["-H", "x-api-key: " + api_key, "-H", "anthropic-version: 2023-06-01"]
    else:
        cmd += ["-H", "Authorization: Bearer " + api_key]
    try:
        p = subprocess.run(cmd, capture_output=True, text=True, timeout=120)
        out = p.stdout
        head, _, code = out.rpartition("\n")
        code = code.strip()
        try:
            data = json.loads(head)
        except Exception:  # noqa: BLE001
            return model, False, "HTTP%s %s" % (code, head[:140].replace("\n", " "))
    except Exception as e:  # noqa: BLE001
        return model, False, "ERR " + type(e).__name__ + " " + str(e)[:80]

    if platform == "anthropic":
        ok = isinstance(data.get("content"), list) and len(data["content"]) > 0
    else:
        ok = bool(data.get("choices"))
    if ok:
        return model, True, "OK"
    err = data.get("error") or {}
    msg = err.get("message") if isinstance(err, dict) else str(err)
    return model, False, (msg or json.dumps(data)[:160]).replace("\n", " ")[:140]


def main():
    accounts = load_accounts()

    only = None
    rounds = 1
    if "--only" in sys.argv:
        only = set(sys.argv[sys.argv.index("--only") + 1].split(","))
    if "--rounds" in sys.argv:
        rounds = int(sys.argv[sys.argv.index("--rounds") + 1])
    idx = {a["id"]: a for a in accounts}

    pairs = []
    if only:
        for item in only:
            acc, model = item.split(":", 1)
            a = idx[int(acc)]
            pairs.append((int(acc), a["platform"], a["base_url"], a["api_key"],
                          model, (a.get("mm") or {}).get(model, model)))
    else:
        for a in accounts:
            for site_model, upstream in (a.get("mm") or {}).items():
                pairs.append((a["id"], a["platform"], a["base_url"], a["api_key"],
                              site_model, upstream))

    tasks = [(p, r) for p in pairs for r in range(rounds)]

    results = {}
    with ThreadPoolExecutor(max_workers=5) as ex:
        futs = {}
        for (acc_id, platform, base_url, key, site_model, upstream), rnd in tasks:
            f = ex.submit(call, base_url, key, platform, upstream)
            futs[f] = (acc_id, site_model, upstream, rnd)
        for f in futs:
            acc_id, site_model, upstream, rnd = futs[f]
            try:
                model, ok, detail = f.result()
            except Exception as e:  # noqa: BLE001
                model, ok, detail = upstream, False, "SUBMIT_ERR " + str(e)[:80]
            results.setdefault(acc_id, []).append((site_model, upstream, ok, detail, rnd))

    for acc_id in sorted(results):
        rows = results[acc_id]
        groups = {}
        for site_model, upstream, ok, detail, rnd in rows:
            g = groups.setdefault((site_model, upstream), [0, 0, []])
            g[1] += 1
            if ok:
                g[0] += 1
            else:
                g[2].append(detail)
        good_total = sum(1 for r in rows if r[2])
        print("=== account %s : %d/%d OK ===" % (acc_id, good_total, len(rows)))
        for (site_model, upstream), (okc, total, errs) in sorted(groups.items()):
            flag = "OK  " if okc == total else ("PART" if okc else "FAIL")
            print("  %s  %-32s -> %-32s %d/%d %s" % (
                flag, site_model, upstream, okc, total,
                "" if okc == total else (errs[0][:100] if errs else "")))

    print("\n=== JSON_OK ===")
    print(json.dumps(
        {str(k): sorted({r[0] for r in v if r[2]}) for k, v in results.items()},
        ensure_ascii=False))


if __name__ == "__main__":
    main()
