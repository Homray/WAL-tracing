package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"waltracer/waltracer"

	"github.com/jackc/pgx/v5"
)

type Metrics struct {
	Tx           int64
	LogicalBytes int64
	SpanBytes    int64
	Ops          map[string]int64
}

type txState struct {
	component    string
	operation    string
	beginLSN     uint64
	contextLSN   uint64
	logicalBytes int64
}

type rateSnapshot struct {
	LogicalBytes int64
	SpanBytes    int64
}

func mustEnvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseLSN(s string) (uint64, error) {
	var hi, lo uint64
	_, err := fmt.Sscanf(s, "%X/%X", &hi, &lo)
	return (hi << 32) | lo, err
}

func isBegin(data string) bool  { return strings.HasPrefix(data, "BEGIN") }
func isCommit(data string) bool { return strings.HasPrefix(data, "COMMIT") }

func isContextMsg(
	data string,
) bool {
	return strings.Contains(data, "prefix: "+waltracer.MsgPrefix)
}

func isDML(data string) bool { return strings.HasPrefix(data, "table ") }

func extractJSON(data string) string {
	idx := strings.Index(data, "content:")
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(data[idx+len("content:"):])
}

func printReport(
	agg map[string]*Metrics,
	prev map[string]rateSnapshot,
	elapsed time.Duration,
) map[string]rateSnapshot {
	fmt.Println()
	fmt.Println(
		"╔═══════════════════════════════════════════════════════════════════════════════════════════════════╗",
	)
	fmt.Println(
		"║                                    WAL Tracing Report                                             ║",
	)
	fmt.Println(
		"╠═══════════════════════════════════════════════════════════════════════════════════════════════════╣",
	)
	fmt.Println(
		"║  Logical(B) — детерм. метрика логических изменений:  Σ len(DML data)                              ║",
	)
	fmt.Println(
		"║  Span(B)    — приближ. оценка физического диапазона WAL: LSN(COMMIT)-LSN(ctx)                     ║",
	)
	fmt.Println(
		"╚═══════════════════════════════════════════════════════════════════════════════════════════════════╝",
	)
	fmt.Println()

	var totalLogical, totalSpan int64
	var totalTx int64
	for _, m := range agg {
		totalLogical += m.LogicalBytes
		totalSpan += m.SpanBytes
		totalTx += m.Tx
	}

	components := make([]string, 0, len(agg))
	for c := range agg {
		components = append(components, c)
	}
	sort.Strings(components)

	sec := elapsed.Seconds()
	if sec <= 0 {
		sec = 1
	}

	next := make(map[string]rateSnapshot)

	fmt.Printf("  %-22s %6s  %10s %7s  %10s %7s  %7s\n",
		"Component", "Tx",
		"Logical(B)", "Log%",
		"Span(B)", "Spn%",
		"Log/Tx",
	)
	fmt.Println("  " + strings.Repeat("─", 85))

	for _, c := range components {
		m := agg[c]

		logPct, spnPct := float64(0), float64(0)
		if totalLogical > 0 {
			logPct = float64(m.LogicalBytes) / float64(totalLogical) * 100
		}
		if totalSpan > 0 {
			spnPct = float64(m.SpanBytes) / float64(totalSpan) * 100
		}

		logPerTx := float64(0)
		if m.Tx > 0 {
			logPerTx = float64(m.LogicalBytes) / float64(m.Tx)
		}

		fmt.Printf("  %-22s %6d  %10d %6.1f%%  %10d %6.1f%%  %7.0f\n",
			c, m.Tx,
			m.LogicalBytes, logPct,
			m.SpanBytes, spnPct,
			logPerTx,
		)

		if len(m.Ops) > 1 {
			ops := make([]string, 0, len(m.Ops))
			for op := range m.Ops {
				ops = append(ops, op)
			}
			sort.Strings(ops)
			for _, op := range ops {
				fmt.Printf("    %-20s %6d\n", op, m.Ops[op])
			}
		}

		next[c] = rateSnapshot{
			LogicalBytes: m.LogicalBytes,
			SpanBytes:    m.SpanBytes,
		}
	}

	fmt.Println("  " + strings.Repeat("─", 85))
	fmt.Printf("  %-22s %6d  %10d          %10d\n",
		"TOTAL", totalTx, totalLogical, totalSpan)

	if totalLogical > 0 {
		overFactor := float64(totalSpan) / float64(totalLogical)
		fmt.Printf("\n  Span/Logical ratio: %.2fx", overFactor)
		switch {
		case overFactor < 1.5:
			fmt.Println("  (низкая конкурентность — span близок к logical)")
		case overFactor < 3.0:
			fmt.Println("  (умеренная конкурентность — span завышен)")
		default:
			fmt.Println("  (высокая конкурентность — span значительно завышен, доверяйте logical)")
		}
	}

	fmt.Println()
	fmt.Printf("  %-22s  %s\n", "Component", "Logical B/s")
	fmt.Println("  " + strings.Repeat("─", 38))
	for _, c := range components {
		m := agg[c]
		delta := m.LogicalBytes - prev[c].LogicalBytes
		if delta < 0 {
			delta = 0
		}
		fmt.Printf("  %-22s  %.0f\n", c, float64(delta)/sec)
	}
	fmt.Println()

	return next
}

func run(ctx context.Context, connStr, slot string, interval time.Duration) error {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx,
		"SELECT pg_create_logical_replication_slot($1, 'test_decoding')", slot)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("create slot %q: %w", slot, err)
	}

	log.Printf("wal-analyzer: consuming slot %q (report every %s)", slot, interval)

	agg := make(map[string]*Metrics)
	active := make(map[uint32]*txState)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	prev := map[string]rateSnapshot{}
	lastReport := time.Now()

	for {
		rows, err := conn.Query(ctx,
			`SELECT lsn, xid, data
			 FROM pg_logical_slot_get_changes($1, NULL, 1000)`,
			slot,
		)
		if err != nil {
			return fmt.Errorf("get_changes: %w", err)
		}

		for rows.Next() {
			var lsnStr string
			var xid uint32
			var data string
			if err := rows.Scan(&lsnStr, &xid, &data); err != nil {
				rows.Close()
				return fmt.Errorf("scan: %w", err)
			}
			lsn, _ := parseLSN(lsnStr)

			switch {
			case isBegin(data):
				active[xid] = &txState{
					component: "unknown",
					beginLSN:  lsn,
				}

			case isContextMsg(data):
				tx := active[xid]
				if tx == nil {
					break
				}
				var msg waltracer.Msg
				if jsonPart := extractJSON(data); jsonPart != "" {
					if json.Unmarshal([]byte(jsonPart), &msg) == nil {
						if msg.Component != "" {
							tx.component = msg.Component
						}
						tx.operation = msg.Name
						tx.contextLSN = lsn
					}
				}

			case isDML(data):
				if tx := active[xid]; tx != nil {
					tx.logicalBytes += int64(len(data))
				}

			case isCommit(data):
				tx := active[xid]
				if tx == nil {
					delete(active, xid)
					break
				}

				comp := tx.component
				if comp == "" {
					comp = "unknown"
				}

				m := agg[comp]
				if m == nil {
					m = &Metrics{Ops: make(map[string]int64)}
					agg[comp] = m
				}
				m.Tx++

				m.LogicalBytes += tx.logicalBytes

				startLSN := tx.contextLSN
				if startLSN == 0 {
					startLSN = tx.beginLSN
				}
				if startLSN > 0 && startLSN <= lsn {
					m.SpanBytes += int64(lsn - startLSN)
				}

				if tx.operation != "" {
					m.Ops[tx.operation]++
				}

				delete(active, xid)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("rows error: %w", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			now := time.Now()
			prev = printReport(agg, prev, now.Sub(lastReport))
			lastReport = now
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func main() {
	connStr := mustEnvDefault("DB_DSN", "postgres://app:app@localhost:5433/appdb?sslmode=disable")
	slot := mustEnvDefault("WAL_SLOT", "wal_tracer_slot")

	intervalSec, _ := strconv.Atoi(os.Getenv("INTERVAL"))
	if intervalSec <= 0 {
		intervalSec = 5
	}
	interval := time.Duration(intervalSec) * time.Second

	log.Printf("wal-analyzer starting (DSN=%s, slot=%s, interval=%s)", connStr, slot, interval)

	ctx := context.Background()
	for {
		if err := run(ctx, connStr, slot, interval); err != nil {
			log.Printf("wal-analyzer error: %v — retrying in 5s", err)
			time.Sleep(5 * time.Second)
		}
	}
}
