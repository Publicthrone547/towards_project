package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var visualCrossingKey = "SKL8Z6DG99ASZ66YWBJHPH3S7"

var geminiAPIKey string

func InitGemini(apiKey string) {
	geminiAPIKey = apiKey
}

type WeatherResponse struct {
	City              string                   `json:"city"`
	Temperature       float64                  `json:"temperature"`
	Conditions        string                   `json:"conditions"`
	AirPurity         int                      `json:"air_purity"`
	RoadTraffic       int                      `json:"road_traffic"`
	CrimeRisks        int                      `json:"crime_risks"`
	LifeComfortIdx    float64                  `json:"life_comfort_index"`
	Date              string                   `json:"date,omitempty"`
	TempMax           float64                  `json:"temp_max,omitempty"`
	TempMin           float64                  `json:"temp_min,omitempty"`
	Humidity          float64                  `json:"humidity,omitempty"`
	WindSpeed         float64                  `json:"wind_speed,omitempty"`
	Hours             []interface{}            `json:"hours,omitempty"`
	AIForecast        string                   `json:"ai_forecast,omitempty"`
	GDPUSD            float64                  `json:"gdp_usd,omitempty"`
	PopulationTotal   int64                    `json:"population_total,omitempty"`
	PopulationDensity float64                  `json:"population_density,omitempty"`
	CityPopulation    int64                    `json:"city_population,omitempty"`
	CityDensity       float64                  `json:"city_density_per_km2,omitempty"`
	Pressure          float64                  `json:"pressure,omitempty"`
	EarthquakeRisk    float64                  `json:"earthquake_risk,omitempty"`
	EarthquakeCount   int                      `json:"earthquake_count,omitempty"`
	EarthquakeMaxMag  float64                  `json:"earthquake_max_mag,omitempty"`
	RecentQuakes      []map[string]interface{} `json:"recent_quakes,omitempty"`
}

func GetWeather(c *gin.Context) {
	city := c.Query("city")
	if city == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "city query param required"})
		return
	}

	if k := os.Getenv("VISUAL_CROSSING_KEY"); k != "" {
		visualCrossingKey = k
	}

	base := "https://weather.visualcrossing.com/VisualCrossingWebServices/rest/services/timeline/"

	dateParam := c.Query("date")
	var u string
	var respDate time.Time

	ensureGeminiKey := func() string {
		if geminiAPIKey != "" {
			return geminiAPIKey
		}
		return os.Getenv("GEMINI_API_KEY")
	}

	if dateParam != "" {
		var parsed time.Time
		var err error
		parsed, err = time.Parse("02-01-2006", dateParam)
		if err != nil {
			parsed, err = time.Parse("2006-01-02", dateParam)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "date must be DD-MM-YYYY or YYYY-MM-DD"})
				return
			}
		}
		respDate = parsed

		u = base + url.PathEscape(city) + "/" + parsed.Format("2006-01-02") + "?unitGroup=metric&include=days&key=" + url.QueryEscape(visualCrossingKey) + "&contentType=json"

		resp, err := http.Get(u)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch from visualcrossing", "detail": err.Error()})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			c.JSON(http.StatusBadGateway, gin.H{"error": "visualcrossing returned non-200", "status": resp.Status})
			return
		}

		var body map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode response", "detail": err.Error()})
			return
		}

		var tempMax, tempMin, hum, wind, pressure float64
		var cond string
		var hours []interface{}
		if days, ok := body["days"].([]interface{}); ok && len(days) > 0 {
			if dm, ok := days[0].(map[string]interface{}); ok {
				if v, ok := dm["tempmax"].(float64); ok {
					tempMax = v
				}
				if v, ok := dm["tempmin"].(float64); ok {
					tempMin = v
				}
				if tempMax == 0 {
					if v, ok := dm["temp"].(float64); ok {
						tempMax = v
					}
				}
				if v, ok := dm["humidity"].(float64); ok {
					hum = v
				}
				if v, ok := dm["windspeed"].(float64); ok {
					wind = v
				}
				if v, ok := dm["pressure"].(float64); ok {
					pressure = v
				} else if pn, ok := dm["pressure"].(json.Number); ok {
					if fv, err := pn.Float64(); err == nil {
						pressure = fv
					}
				}
				if cnd, ok := dm["conditions"].(string); ok {
					cond = cnd
				}
				if h, ok := dm["hours"].([]interface{}); ok {
					hours = h
				}
			}
		} else {
			c.JSON(http.StatusBadGateway, gin.H{"error": "visualcrossing returned no day data for that date"})
			return
		}

		airScore := getAirScore(city)
		trafficScore := getTrafficScore(city)
		crimeScore := getCrimeScore(city)

		tempForIndex := tempMax
		tempDiff := tempForIndex - 21.0
		if tempDiff < 0 {
			tempDiff = -tempDiff
		}
		tempPenalty := tempDiff * 5.0
		tempScore := 100.0 - tempPenalty
		if tempScore < 0 {
			tempScore = 0
		}
		if tempScore > 100 {
			tempScore = 100
		}

		trafficComfort := 100 - trafficScore
		if trafficComfort < 0 {
			trafficComfort = 0
		}
		crimeComfort := 100 - crimeScore
		if crimeComfort < 0 {
			crimeComfort = 0
		}

		total := (tempScore + float64(airScore) + float64(trafficComfort) + float64(crimeComfort)) / 4.0
		if total < 0 {
			total = 0
		}
		if total > 100 {
			total = 100
		}

		out := WeatherResponse{
			City:           city,
			Temperature:    tempMax,
			Conditions:     cond,
			AirPurity:      airScore,
			RoadTraffic:    trafficScore,
			CrimeRisks:     crimeScore,
			LifeComfortIdx: total,
			Date:           respDate.Format("02-01-2006"),
			TempMax:        tempMax,
			TempMin:        tempMin,
			Humidity:       hum,
			WindSpeed:      wind,
			Pressure:       pressure,
			Hours:          hours,
		}

		// Try to fetch country stats
		country := getCountryFromBody(body)
		if country != "" {
			if gdp, pop, dens, err := fetchCountryStats(country); err == nil {
				out.GDPUSD = gdp
				out.PopulationTotal = pop
				out.PopulationDensity = dens
			}
			if cp, carea, err := fetchCityStats(city, country); err == nil {
				out.CityPopulation = cp
				if carea > 0 {
					out.CityDensity = float64(cp) / carea
				}
			}
		}

		// Earthquake data: use lat/lon if available
		if latv, lok := body["latitude"].(float64); lok {
			if lonv, lok2 := body["longitude"].(float64); lok2 {
				if risk, cnt, maxm, rec, err := fetchEarthquakeRisk(latv, lonv, 100, 30); err == nil {
					out.EarthquakeRisk = risk
					out.EarthquakeCount = cnt
					out.EarthquakeMaxMag = maxm
					out.RecentQuakes = rec
				}
			}
		}

		now := time.Now().UTC()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		parsedDate := time.Date(respDate.Year(), respDate.Month(), respDate.Day(), 0, 0, 0, 0, time.UTC)
		if parsedDate.Before(today) {
			c.JSON(http.StatusOK, out)
			return
		}

		apiKey := ensureGeminiKey()
		if apiKey == "" {
			c.JSON(http.StatusOK, out)
			return
		}

		instruction := "You are an assistant that generates a short weather forecast and a brief day comfort summary in English. " +
			"You MUST use and PRESERVE the numeric values provided in the prompt exactly, and insert them into a readable sentence. " +
			"Response format: one short line (not JSON) containing the temperature (°C), main conditions, humidity (%) and wind speed (m/s), " +
			"plus a short tip (what to take/how to dress). The numeric values in the sentence must exactly match those in the prompt."

		prompt := fmt.Sprintf("City: %s\nDate: %s (Year: %d)\nTemperature_max: %.1f\nHumidity: %.1f\nWindSpeed: %.1f\nAirPurity: %d\nRoadTraffic: %d\nCrimeRisks: %d\nLifeComfortIndex: %.1f\nConditions: %s",
			city, respDate.Format("2006-01-02"), respDate.Year(), tempMax, hum, wind, airScore, trafficScore, crimeScore, total, cond)

		aiText, err := askGemini(apiKey, instruction, prompt)
		if err != nil {
			out.AIForecast = fmt.Sprintf("gemini error: %v", err)
			c.JSON(http.StatusOK, out)
			return
		}

		out.AIForecast = aiText
		c.JSON(http.StatusOK, out)
		return
	}

	u = base + url.PathEscape(city) + "?unitGroup=metric&include=current&key=" + url.QueryEscape(visualCrossingKey) + "&contentType=json"
	resp, err := http.Get(u)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch from visualcrossing", "detail": err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": "visualcrossing returned non-200", "status": resp.Status})
		return
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode response", "detail": err.Error()})
		return
	}

	cityName, _ := body["resolvedAddress"].(string)
	temp := 0.0
	cond := ""
	var tempMax float64
	var tempMin float64
	var hum float64
	var wind float64
	var pressure float64
	var hours []interface{}

	if curr, ok := body["currentConditions"].(map[string]interface{}); ok {
		if t, ok := curr["temp"].(float64); ok {
			temp = t
		} else if tf, ok := curr["temp"].(json.Number); ok {
			if fv, err := tf.Float64(); err == nil {
				temp = fv
			}
		}
		if cnd, ok := curr["conditions"].(string); ok {
			cond = cnd
		}
		if v, ok := curr["humidity"].(float64); ok {
			hum = v
		}
		if v, ok := curr["windspeed"].(float64); ok {
			wind = v
		}
		if v, ok := curr["pressure"].(float64); ok {
			pressure = v
		} else if pn, ok := curr["pressure"].(json.Number); ok {
			if fv, err := pn.Float64(); err == nil {
				pressure = fv
			}
		}
		if h, ok := curr["hours"].([]interface{}); ok {
			hours = h
		}
	}

	if days, ok := body["days"].([]interface{}); ok && len(days) > 0 {
		if dm, ok := days[0].(map[string]interface{}); ok {
			if v, ok := dm["tempmax"].(float64); ok {
				tempMax = v
			}
			if v, ok := dm["tempmin"].(float64); ok {
				tempMin = v
			}
			if temp == 0 {
				if tv, ok := dm["temp"].(float64); ok {
					temp = tv
				} else {
					if tempMax != 0 || tempMin != 0 {
						temp = (tempMax + tempMin) / 2.0
					}
				}
			}
			if cnd, ok := dm["conditions"].(string); ok && cond == "" {
				cond = cnd
			}
			if h, ok := dm["hours"].([]interface{}); ok && len(hours) == 0 {
				hours = h
			}
		}
	}

	tempToReturn := temp
	if tempMax != 0 {
		tempToReturn = tempMax
	}

	airScore := getAirScore(city)
	trafficScore := getTrafficScore(city)
	crimeScore := getCrimeScore(city)

	tempForIndex := tempToReturn
	tempDiff := tempForIndex - 21.0
	if tempDiff < 0 {
		tempDiff = -tempDiff
	}
	tempPenalty := tempDiff * 5.0
	tempScore := 100.0 - tempPenalty
	if tempScore < 0 {
		tempScore = 0
	}
	if tempScore > 100 {
		tempScore = 100
	}

	trafficComfort := 100 - trafficScore
	if trafficComfort < 0 {
		trafficComfort = 0
	}
	crimeComfort := 100 - crimeScore
	if crimeComfort < 0 {
		crimeComfort = 0
	}

	total := (tempScore + float64(airScore) + float64(trafficComfort) + float64(crimeComfort)) / 4.0
	if total < 0 {
		total = 0
	}
	if total > 100 {
		total = 100
	}

	out := WeatherResponse{
		City:           cityName,
		Temperature:    tempToReturn,
		Conditions:     cond,
		AirPurity:      airScore,
		RoadTraffic:    trafficScore,
		CrimeRisks:     crimeScore,
		LifeComfortIdx: total,
		Date:           time.Now().Format("02-01-2006"),
		TempMax:        tempMax,
		TempMin:        tempMin,
		Humidity:       hum,
		WindSpeed:      wind,
		Pressure:       pressure,
		Hours:          hours,
	}

	country := getCountryFromBody(body)
	if country != "" {
		if gdp, pop, dens, err := fetchCountryStats(country); err == nil {
			out.GDPUSD = gdp
			out.PopulationTotal = pop
			out.PopulationDensity = dens
		}
		if cp, carea, err := fetchCityStats(cityName, country); err == nil {
			out.CityPopulation = cp
			if carea > 0 {
				out.CityDensity = float64(cp) / carea
			}
		}
	}

	if latv, lok := body["latitude"].(float64); lok {
		if lonv, lok2 := body["longitude"].(float64); lok2 {
			if risk, cnt, maxm, rec, err := fetchEarthquakeRisk(latv, lonv, 100, 30); err == nil {
				out.EarthquakeRisk = risk
				out.EarthquakeCount = cnt
				out.EarthquakeMaxMag = maxm
				out.RecentQuakes = rec
			}
		}
	}

	apiKey := ensureGeminiKey()
	if apiKey == "" {
		c.JSON(http.StatusOK, out)
		return
	}

	instruction := "You are an assistant that generates a short weather forecast and a brief day comfort summary in English. " +
		"You MUST use and PRESERVE the numeric values provided in the prompt exactly, and insert them into a readable sentence. " +
		"Response format: one short line (not JSON) containing the temperature (°C), main conditions, humidity (%) and wind speed (m/s), " +
		"plus a short tip (what to take/how to dress). The numeric values in the sentence must exactly match those in the prompt."

	prompt := fmt.Sprintf("City: %s\nDate: %s (Year: %d)\nTemperature_max: %.1f\nHumidity: %.1f\nWindSpeed: %.1f\nAirPurity: %d\nRoadTraffic: %d\nCrimeRisks: %d\nLifeComfortIndex: %.1f\nConditions: %s",
		cityName, time.Now().Format("2006-01-02"), time.Now().Year(), tempToReturn, hum, wind, airScore, trafficScore, crimeScore, total, cond)

	aiText, err := askGemini(apiKey, instruction, prompt)
	if err != nil {
		out.AIForecast = fmt.Sprintf("gemini error: %v", err)
		c.JSON(http.StatusOK, out)
		return
	}

	out.AIForecast = aiText
	c.JSON(http.StatusOK, out)
}

func fetchCountryFromResolvedAddress(addr string) string {
	parts := strings.Split(addr, ",")
	if len(parts) == 0 {
		return ""
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	return last
}

func fetchCountryStats(country string) (float64, int64, float64, error) {
	if country == "" {
		return 0, 0, 0, fmt.Errorf("country empty")
	}

	rcURL := "https://restcountries.com/v3.1/name/" + url.PathEscape(country)
	resp, err := http.Get(rcURL)
	if err != nil {
		return 0, 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, 0, fmt.Errorf("restcountries returned %s", resp.Status)
	}
	var rc []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rc); err != nil {
		return 0, 0, 0, err
	}
	if len(rc) == 0 {
		return 0, 0, 0, fmt.Errorf("no country data")
	}
	var population int64
	var area float64
	if popv, ok := rc[0]["population"].(float64); ok {
		population = int64(popv)
	}
	if areaV, ok := rc[0]["area"].(float64); ok {
		area = areaV
	}

	var gdp float64
	if cca3, ok := rc[0]["cca3"].(string); ok && cca3 != "" {
		wbURL := fmt.Sprintf("https://api.worldbank.org/v2/country/%s/indicator/NY.GDP.MKTP.CD?format=json&per_page=1", strings.ToLower(cca3))
		wbResp, err := http.Get(wbURL)
		if err == nil {
			defer wbResp.Body.Close()
			if wbResp.StatusCode == http.StatusOK {
				var wb []interface{}
				if err := json.NewDecoder(wbResp.Body).Decode(&wb); err == nil && len(wb) > 1 {
					if series, ok := wb[1].([]interface{}); ok && len(series) > 0 {
						if first, ok := series[0].(map[string]interface{}); ok {
							if val, ok := first["value"].(float64); ok {
								gdp = val
							}
						}
					}
				}
			}
		}
	}

	var density float64
	if area > 0 && population > 0 {
		density = float64(population) / area
	}

	return gdp, population, density, nil
}

func fetchCityStats(city, country string) (int64, float64, error) {
	if city == "" {
		return 0, 0, fmt.Errorf("city empty")
	}
	q := url.QueryEscape(city + ", " + country)
	nomURL := "https://nominatim.openstreetmap.org/search?format=json&limit=1&q=" + q + "&addressdetails=1&extratags=1"
	req, _ := http.NewRequest("GET", nomURL, nil)
	req.Header.Set("User-Agent", "towards_project/1.0")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("nominatim returned %s", resp.Status)
	}
	var res []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, 0, err
	}
	if len(res) == 0 {
		return 0, 0, fmt.Errorf("no nominatim result")
	}
	var pop int64
	var area float64
	if ex, ok := res[0]["extratags"].(map[string]interface{}); ok {
		if pv, ok := ex["population"].(string); ok {
			var p64 int64
			if _, err := fmt.Sscan(pv, &p64); err == nil {
				pop = p64
			}
		}
	}
	return pop, area, nil
}

func getCountryFromBody(body map[string]interface{}) string {
	if addr, ok := body["resolvedAddress"].(string); ok && addr != "" {
		return fetchCountryFromResolvedAddress(addr)
	}
	lat, latOk := body["latitude"].(float64)
	lon, lonOk := body["longitude"].(float64)
	if latOk && lonOk {
		if c := nominatimReverse(lat, lon); c != "" {
			return c
		}
	}
	return ""
}

func nominatimReverse(lat, lon float64) string {
	url := fmt.Sprintf("https://nominatim.openstreetmap.org/reverse?format=json&lat=%.6f&lon=%.6f&zoom=3&addressdetails=1", lat, lon)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "towards_project/1.0")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var res map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return ""
	}
	if addr, ok := res["address"].(map[string]interface{}); ok {
		if country, ok := addr["country"].(string); ok {
			return country
		}
	}
	return ""
}

func askGemini(apiKey, instruction, promt string) (string, error) {
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]string{
					{"text": instruction},
					{"text": promt},
				},
			},
		},
	}

	data, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-goog-api-key", apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var res struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", err
	}

	if len(res.Candidates) > 0 && len(res.Candidates[0].Content.Parts) > 0 {
		return res.Candidates[0].Content.Parts[0].Text, nil
	}
	return "No response", nil
}

func getAirScore(city string) int {
	if city == "" {
		return 50
	}

	v := (len(city)*37 + 17) % 101
	if v < 20 {
		v += 30
	}
	return v
}

func getTrafficScore(city string) int {
	v := (len(city)*53 + 11) % 101
	return v
}

func getCrimeScore(city string) int {
	v := (len(city)*73 + 29) % 101
	return v
}
