-- Read-only. Run on the target DB only when its owner authorizes read access.
SELECT s.schemaname,s.relname,s.indexrelname,s.idx_scan,pg_relation_size(s.indexrelid) AS index_bytes,pg_get_indexdef(s.indexrelid) AS definition,d.stats_reset
FROM pg_stat_user_indexes s CROSS JOIN (SELECT stats_reset FROM pg_stat_database WHERE datname=current_database()) d
WHERE s.relname='usage_logs' ORDER BY s.indexrelname;

-- Exact B-tree prefix candidates only. Different expressions, directions,
-- predicates, opclasses and INCLUDE columns must not be treated as duplicates.
WITH defs AS (
 SELECT i.*,ic.relname AS index_name,am.amname,pg_get_expr(i.indpred,i.indrelid) AS predicate,
 ARRAY(SELECT pg_get_indexdef(i.indexrelid,k,true) FROM generate_series(1,i.indnkeyatts) k) AS keys
 FROM pg_index i JOIN pg_class t ON t.oid=i.indrelid JOIN pg_class ic ON ic.oid=i.indexrelid JOIN pg_am am ON am.oid=ic.relam
 WHERE t.relname='usage_logs' AND am.amname='btree' AND i.indisvalid AND i.indisready AND i.indnatts=i.indnkeyatts
) SELECT a.index_name AS candidate,b.index_name AS covering_index,a.keys,b.keys AS covering_keys,
 pg_get_indexdef(a.indexrelid) AS rollback_definition,
 EXISTS(SELECT 1 FROM pg_constraint WHERE conindid=a.indexrelid) AS constraint_owned
FROM defs a JOIN defs b ON a.indrelid=b.indrelid AND a.indexrelid<>b.indexrelid
WHERE NOT a.indisunique AND NOT a.indisprimary AND a.indnkeyatts<b.indnkeyatts
 AND a.predicate IS NOT DISTINCT FROM b.predicate
 AND a.keys=b.keys[1:a.indnkeyatts]
 AND (a.indclass::oid[])[0:a.indnkeyatts-1]=(b.indclass::oid[])[0:a.indnkeyatts-1]
 AND (a.indcollation::oid[])[0:a.indnkeyatts-1]=(b.indcollation::oid[])[0:a.indnkeyatts-1]
 AND (a.indoption::smallint[])[0:a.indnkeyatts-1]=(b.indoption::smallint[])[0:a.indnkeyatts-1]
ORDER BY a.index_name,b.index_name;
