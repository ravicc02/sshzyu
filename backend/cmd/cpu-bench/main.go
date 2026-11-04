// cpu-bench operates only on the explicitly named local cpu_bench database.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "github.com/lib/pq"
	"log"
	"net/url"
	"os"
	"time"
)

func main() {
	dsn := os.Getenv("CPU_BENCH_DSN")
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.Path != "/cpu_bench" {
		log.Fatal("CPU_BENCH_DSN must name local /cpu_bench")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := timezone.Init("Asia/Shanghai"); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	repo := repository.NewDashboardAggregationRepository(db).(service.GroupUsageRollupRepository)
	for i := 0; i < 3; i++ {
		started := time.Now()
		if err := repo.SyncGroupUsageRollups(ctx, time.Now()); err != nil {
			log.Fatal(err)
		}
		var w string
		var rows int64
		if err := db.QueryRowContext(ctx, "SELECT closed_before::text FROM usage_group_rollup_state WHERE id=1").Scan(&w); err != nil {
			log.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM usage_group_daily_rollups").Scan(&rows); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("run=%d closed_before=%s buckets=%d elapsed=%s\n", i+1, w, rows, time.Since(started))
	}
}
