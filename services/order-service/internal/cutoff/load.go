package cutoff

import (
	"encoding/csv"
	"os"
	"strconv"
	"time"
)

func FromCSV(path string) (*Calendar, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var days []Day
	for i, row := range rows {
		if i == 0 || len(row) < 2 {
			continue
		}
		d, err := time.Parse("2006-01-02", row[0])
		if err != nil {
			continue
		}
		op, _ := strconv.Atoi(row[1])
		days = append(days, Day{Date: d, IsOperating: op == 1})
	}
	return Load(days), nil
}
