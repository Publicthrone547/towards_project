package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"
)

type quakeEvent struct {
	Time     time.Time
	Mag      float64
	Place    string
	Distance float64
}

func fetchEarthquakeRisk(lat, lon float64, radiusKm int, periodYears int) (float64, int, float64, []map[string]interface{}, error) {
	end := time.Now().UTC()
	start := end.AddDate(-periodYears, 0, 0)
	url := fmt.Sprintf("https://earthquake.usgs.gov/fdsnws/event/1/query.geojson?starttime=%s&endtime=%s&latitude=%.6f&longitude=%.6f&maxradiuskm=%d&minmagnitude=3&format=geojson",
		start.Format("2006-01-02"), end.Format("2006-01-02"), lat, lon, radiusKm)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, 0, nil, fmt.Errorf("usgs returned %s", resp.Status)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, 0, 0, nil, err
	}

	features, _ := data["features"].([]interface{})
	count := 0
	maxMag := 0.0
	recent := []map[string]interface{}{}
	for i := len(features) - 1; i >= 0; i-- {
		if f, ok := features[i].(map[string]interface{}); ok {
			props, _ := f["properties"].(map[string]interface{})
			mag := 0.0
			if mv, ok := props["mag"].(float64); ok {
				mag = mv
			}
			if mag >= 3.0 {
				count++
				if mag > maxMag {
					maxMag = mag
				}
				if len(recent) < 10 {
					r := map[string]interface{}{
						"time":  props["time"],
						"mag":   mag,
						"place": props["place"],
					}
					recent = append(recent, r)
				}
			}
		}
	}


	var totalWeightedEnergy float64
	now := time.Now()
	for i := range features {
		if f, ok := features[i].(map[string]interface{}); ok {
			props, _ := f["properties"].(map[string]interface{})
			mag := 0.0
			if mv, ok := props["mag"].(float64); ok {
				mag = mv
			}
			if mag < 0.0 {
				continue
			}
			timeVal := int64(0)
			if tv, ok := props["time"].(float64); ok {
				timeVal = int64(tv)
			}
			t := time.Unix(0, timeVal*int64(time.Millisecond))
			days := now.Sub(t).Hours() / 24.0
			decay := math.Exp(-days / 365.0)
			energy := math.Pow(10.0, 1.5*mag)
			totalWeightedEnergy += energy * decay
		}
	}

	refEnergy := math.Pow(10.0, 1.5*7.0)
	score := math.Log10(totalWeightedEnergy/refEnergy+1.0) * 50.0
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score, count, maxMag, recent, nil
}
