package main

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"slices"
	"strconv"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

type Metric struct {
	AvgBlockLatency   string `json:"AvgBlockLatency"`
	AvgNetworkLatency string `json:"AvgNetworkLatency"`
	Block             int64  `json:"Block"`
	Type              string `json:"Type"`
}

func main() {
	file, _ := os.Open("metrics.log")
	defer file.Close()
	data := make(map[int64]map[string]Metric)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var m Metric
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			log.Println("unmarshal error:", err)
			continue
		}
		if _, ok := data[m.Block]; !ok {
			data[m.Block] = make(map[string]Metric)
		}
		data[m.Block][m.Type] = m
	}

	dAvgBlockLatPoints := make(plotter.XYs, 0)
	dAvgNetLatPoints := make(plotter.XYs, 0)
	lAvgBlockLatPoints := make(plotter.XYs, 0)
	lAvgNetLatPoints := make(plotter.XYs, 0)

	for block, m := range data {
		if d, ok := m["dedicated"]; ok {
			lat, _ := strconv.ParseFloat(d.AvgBlockLatency, 64)
			dAvgBlockLatPoints = append(dAvgBlockLatPoints, plotter.XY{X: float64(block), Y: lat})
			lat, _ = strconv.ParseFloat(d.AvgNetworkLatency, 64)
			dAvgNetLatPoints = append(dAvgNetLatPoints, plotter.XY{X: float64(block), Y: lat})
		}
		if l, ok := m["laser"]; ok {
			lat, err := strconv.ParseFloat(l.AvgBlockLatency, 64)
			if err != nil {
				log.Fatal(err)
			}
			lAvgBlockLatPoints = append(lAvgBlockLatPoints, plotter.XY{X: float64(block), Y: lat})
			lat, _ = strconv.ParseFloat(l.AvgNetworkLatency, 64)
			lAvgNetLatPoints = append(lAvgNetLatPoints, plotter.XY{X: float64(block), Y: lat})
		}
	}
	slices.SortFunc(dAvgBlockLatPoints, func(a plotter.XY, b plotter.XY) int {
		if a.X < b.X {
			return -1
		}
		if a.X > b.X {
			return 1
		}
		return 0
	})
	slices.SortFunc(lAvgBlockLatPoints, func(a plotter.XY, b plotter.XY) int {
		if a.X < b.X {
			return -1
		}
		if a.X > b.X {
			return 1
		}
		return 0
	})
	slices.SortFunc(dAvgNetLatPoints, func(a plotter.XY, b plotter.XY) int {
		if a.X < b.X {
			return -1
		}
		if a.X > b.X {
			return 1
		}
		return 0
	})
	slices.SortFunc(lAvgNetLatPoints, func(a plotter.XY, b plotter.XY) int {
		if a.X < b.X {
			return -1
		}
		if a.X > b.X {
			return 1
		}
		return 0
	})

	p := plot.New()
	p.Title.Text = "Helius Latency Comparison"
	p.X.Label.Text = "Block"
	p.Y.Label.Text = "Latency (ms)"
	p.Legend.Top = true

	err := plotutil.AddLinePoints(p,
		"D-AvgBlockLatency", dAvgBlockLatPoints,
		"D-AvgNetkLatency", dAvgNetLatPoints,
		"L-AvgBlcokLatency", lAvgBlockLatPoints,
		"L-AvgNetkLatency", lAvgNetLatPoints)
	if err != nil {
		panic(err)
	}

	// 保存为 PNG
	if err := p.Save(10*vg.Inch, 4*vg.Inch, "metrics.png"); err != nil {
		panic(err)
	}
}
