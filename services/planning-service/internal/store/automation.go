package store

import (
	"context"
	"github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/automation"
	"time"
)

func (p Postgres) AutomationSnapshot(ctx context.Context, at time.Time) (automation.Snapshot, error) {
	out := automation.Snapshot{AsOf: at, Items: []automation.Deferred{}}
	if err := p.Pool.QueryRow(ctx, `SELECT since FROM planning.automation_history_start`).Scan(&out.HistorySince); err != nil {
		return out, err
	}
	if at.Before(out.HistorySince) {
		return out, nil
	}
	rows, err := p.Pool.Query(ctx, `WITH latest AS (
 SELECT DISTINCT ON(order_id) order_id,outlet_id,delivery_date,state
 FROM planning.automation_order_history WHERE recorded_at<=$1
 ORDER BY order_id,delivery_date DESC,recorded_at DESC,id DESC
 ), counts AS (
 SELECT outlet_id,count(DISTINCT delivery_date)::int AS n FROM planning.automation_order_history
 WHERE state='deferred' AND recorded_at<=$1 AND recorded_at>$1-interval '28 days' GROUP BY outlet_id
 ) SELECT l.order_id,l.outlet_id,l.delivery_date::text,COALESCE(c.n,1)
 FROM latest l LEFT JOIN counts c USING(outlet_id) WHERE l.state='deferred' ORDER BY l.order_id`, at)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var d automation.Deferred
		if err = rows.Scan(&d.OrderID, &d.OutletID, &d.Date, &d.Count); err != nil {
			return out, err
		}
		out.Items = append(out.Items, d)
	}
	return out, rows.Err()
}
