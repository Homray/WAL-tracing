package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Metrics struct {
	Tx       int64
	WalBytes int64
}

type ContextMsg struct {
	Component string `json:"component"`
	Operation string `json:"operation"`
	Entity    string `json:"entity"`
	TraceID   string `json:"trace_id"`
}

func parseLSN(s string) (uint64, error) {
	var hi, lo uint64
	_, err := fmt.Sscanf(s, "%X/%X", &hi, &lo)
	if err != nil {
		return 0, err
	}
	return (hi << 32) + lo, nil
}

func isBegin(data string) bool  { return strings.HasPrefix(data, "BEGIN") }
func isCommit(data string) bool { return strings.HasPrefix(data, "COMMIT") }

func isContextMsg(data string) bool {
	return strings.Contains(data, "prefix: app_context")
}

func extractJSONContent(data string) string {
	idx := strings.Index(data, "content:")
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(data[idx+len("content:"):])
}

type RateSnapshot struct {
	WalBytes int64
}

func printReport(
	agg map[string]*Metrics,
	prev map[string]RateSnapshot,
	elapsed time.Duration,
) map[string]RateSnapshot {
	fmt.Println()
	fmt.Println("=== WAL tracing report ===")
	fmt.Println()

	var totalWal int64
	for _, m := range agg {
		totalWal += m.WalBytes
	}

	components := make([]string, 0, len(agg))
	for c := range agg {
		components = append(components, c)
	}
	sort.Strings(components)

	fmt.Printf(
		"%-17s %8s %12s %14s %15s %7s\n",
		"Component", "Tx", "WAL(MiB)", "WAL/Tx(KiB)", "WAL/sec(KiB)", "WAL%",
	)

	fmt.Println(strings.Repeat("-", 78))

	nextPrev := make(map[string]RateSnapshot)

	sec := elapsed.Seconds()
	if sec <= 0 {
		sec = 1
	}

	for _, c := range components {
		m := agg[c]

		walMiB := float64(m.WalBytes) / (1024 * 1024)

		var walPerTx float64
		if m.Tx > 0 {
			walPerTx = (float64(m.WalBytes) / 1024) / float64(m.Tx)
		}

		var walPct float64
		if totalWal > 0 {
			walPct = float64(m.WalBytes) / float64(totalWal) * 100
		}

		prevSnap := prev[c]
		delta := m.WalBytes - prevSnap.WalBytes
		if delta < 0 {
			delta = 0
		}

		walPerSec := (float64(delta) / 1024) / sec

		fmt.Printf(
			"%-17s %8d %12.2f %14.2f %14.1f %7.2f%%\n",
			c, m.Tx, walMiB, walPerTx, walPerSec, walPct,
		)

		nextPrev[c] = RateSnapshot{WalBytes: m.WalBytes}
	}

	fmt.Println()
	return nextPrev
}

func main() {
	connStr := os.Getenv("DB_DSN")
	if connStr == "" {
		connStr = "postgres://app:app@localhost:5433/appdb?sslmode=disable"
	}

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(ctx)

	slot := "wal_tracing_slot"

	_, err = conn.Exec(ctx,
		`SELECT pg_create_logical_replication_slot($1, 'test_decoding')`,
		slot,
	)

	if err != nil && !strings.Contains(err.Error(), "already exists") {
		log.Fatal(err)
	}

	log.Println("wal-tracer started")

	agg := make(map[string]*Metrics)
	currentComponentByXID := make(map[uint32]string)
	beginLSNByXID := make(map[uint32]uint64)

	reportEvery := 5 * time.Second
	ticker := time.NewTicker(reportEvery)
	defer ticker.Stop()

	prevRates := map[string]RateSnapshot{}
	lastReport := time.Now()

	for {

		rows, err := conn.Query(ctx,
			`SELECT lsn, xid, data
			 FROM pg_logical_slot_get_changes($1, NULL, 1000)`,
			slot,
		)
		if err != nil {
			log.Fatal(err)
		}

		for rows.Next() {

			var lsnStr string
			var xid uint32
			var data string

			if err := rows.Scan(&lsnStr, &xid, &data); err != nil {
				log.Fatal(err)
			}

			lsn, _ := parseLSN(lsnStr)

			switch {

			case isBegin(data):
				beginLSNByXID[xid] = lsn
				currentComponentByXID[xid] = "unknown"

			case isContextMsg(data):

				jsonPart := extractJSONContent(data)

				var msg ContextMsg
				if jsonPart != "" && json.Unmarshal([]byte(jsonPart), &msg) == nil {
					if msg.Component != "" {
						currentComponentByXID[xid] = msg.Component
					}
				}

			case isCommit(data):

				comp := currentComponentByXID[xid]
				if comp == "" {
					comp = "unknown"
				}

				m := agg[comp]
				if m == nil {
					m = &Metrics{}
					agg[comp] = m
				}

				m.Tx++

				if begin, ok := beginLSNByXID[xid]; ok && begin <= lsn {
					m.WalBytes += int64(lsn - begin)
				}

				delete(currentComponentByXID, xid)
				delete(beginLSNByXID, xid)
			}
		}

		rows.Close()

		select {
		case <-ticker.C:

			now := time.Now()
			prevRates = printReport(agg, prevRates, now.Sub(lastReport))
			lastReport = now

		default:
			time.Sleep(800 * time.Millisecond)
		}
	}
}
