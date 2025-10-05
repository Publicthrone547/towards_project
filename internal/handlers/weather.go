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
	"math"
	"time"

	"github.com/gin-gonic/gin"
)

var visualCrossingKey = "SKL8Z6DG99ASZ66YWBJHPH3S7"

var geminiAPIKey string

func InitGemini(apiKey string) {
	geminiAPIKey = apiKey
}

type WeatherResponse struct {
	City              string        `json:"city"`
	Temperature       float64       `json:"temperature"`
	Conditions        string        `json:"conditions"`
	AirPurity         int           `json:"air_purity"`
	RoadTraffic       int           `json:"road_traffic"`
	CrimeRisks        int           `json:"crime_risks"`
	LifeComfortIdx    float64       `json:"life_comfort_index"`
	Date              string        `json:"date,omitempty"`
	TempMax           float64       `json:"temp_max,omitempty"`
	TempMin           float64       `json:"temp_min,omitempty"`
	Humidity          float64       `json:"humidity,omitempty"`
	WindSpeed         float64       `json:"wind_speed,omitempty"`
	Hours             []interface{} `json:"hours,omitempty"`
	AIForecast        string        `json:"ai_forecast,omitempty"`
	GDPUSD            float64       `json:"gdp_usd,omitempty"`
	PopulationTotal   int64         `json:"population_total,omitempty"`
	PopulationDensity float64       `json:"population_density,omitempty"`
	CityPopulation    int64         `json:"city_population,omitempty"`
	CityDensity       float64       `json:"city_density_per_km2,omitempty"`
	EconomyIndex      float64       `json:"economy_index,omitempty"`
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

		var tempMax, tempMin, hum, wind float64
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

		var gdp float64
		var population int64

		out := WeatherResponse{
			City:           city,
			Temperature:    tempMax,
			Conditions:     cond,
			AirPurity:      airScore,
			RoadTraffic:    trafficScore,
			CrimeRisks:     crimeScore,
			Date:           respDate.Format("02-01-2006"),
			TempMax:        tempMax,
			TempMin:        tempMin,
			Humidity:       hum,
			WindSpeed:      wind,
			Hours:          hours,
		}

		country := getCountryFromBody(body)
		if country != "" {
			if ggdp, pop, dens, err := fetchCountryStats(country); err == nil {
				out.GDPUSD = ggdp
				out.PopulationTotal = pop
				out.PopulationDensity = dens
				gdp = ggdp
				population = pop
				if pop > 0 {
					gdpPerCap := ggdp / float64(pop)
					idx := math.Log10(gdpPerCap+1.0) * 20.0
					if idx < 0 {
						idx = 0
					}
					if idx > 100 {
						idx = 100
					}
					out.EconomyIndex = idx
				}
			}
			if cp, carea, err := fetchCityStats(city, country); err == nil {
				out.CityPopulation = cp
				if carea > 0 {
					out.CityDensity = float64(cp) / carea
				}
			}
		}

		out.LifeComfortIdx = computeLifeComfortIndex(tempMax, hum, wind, airScore, trafficScore, crimeScore, gdp, population)

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
			city, respDate.Format("2006-01-02"), respDate.Year(), tempMax, hum, wind, airScore, trafficScore, crimeScore, out.LifeComfortIdx, cond)

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

	var gdp float64
	var population int64

	tempForIndex := tempToReturn

	out := WeatherResponse{
		City:           cityName,
		Temperature:    tempToReturn,
		Conditions:     cond,
		AirPurity:      airScore,
		RoadTraffic:    trafficScore,
		CrimeRisks:     crimeScore,
		Date:           time.Now().Format("02-01-2006"),
		TempMax:        tempMax,
		TempMin:        tempMin,
		Humidity:       hum,
		WindSpeed:      wind,
		Hours:          hours,
	}

	country := getCountryFromBody(body)
	if country != "" {
		if ggdp, pop, dens, err := fetchCountryStats(country); err == nil {
			out.GDPUSD = ggdp
			out.PopulationTotal = pop
			out.PopulationDensity = dens
			gdp = ggdp
			population = pop
			if pop > 0 {
				gdpPerCap := ggdp / float64(pop)
				idx := math.Log10(gdpPerCap+1.0) * 20.0
				if idx < 0 {
					idx = 0
				}
				if idx > 100 {
					idx = 100
				}
				out.EconomyIndex = idx
			}
		}
		if cp, carea, err := fetchCityStats(cityName, country); err == nil {
			out.CityPopulation = cp
			if carea > 0 {
				out.CityDensity = float64(cp) / carea
			}
		}
	}

	out.LifeComfortIdx = computeLifeComfortIndex(tempForIndex, hum, wind, airScore, trafficScore, crimeScore, gdp, population)

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
		cityName, time.Now().Format("2006-01-02"), time.Now().Year(), tempToReturn, hum, wind, airScore, trafficScore, crimeScore, out.LifeComfortIdx, cond)


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
		wbURL := fmt.Sprintf("https://api.worldbank.org/v2/country/%s/indicator/NY.GDP.MKTP.CD?format=json&per_page=1000", strings.ToLower(cca3))
		wbResp, err := http.Get(wbURL)
		if err == nil {
			defer wbResp.Body.Close()
			if wbResp.StatusCode == http.StatusOK {
				var wb []interface{}
				if err := json.NewDecoder(wbResp.Body).Decode(&wb); err == nil && len(wb) > 1 {
					if series, ok := wb[1].([]interface{}); ok && len(series) > 0 {
						for _, si := range series {
							if item, ok := si.(map[string]interface{}); ok {
								if val, exists := item["value"]; exists && val != nil {
									if vf, ok := val.(float64); ok {
										gdp = vf
										break
									}
								}
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
			// try parse
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

func computeLifeComfortIndex(temp, humidity, wind float64, airScore, trafficScore, crimeScore int, gdp float64, population int64) float64 {
	tempDiff := math.Abs(temp - 21.0)
	tempScore := 100.0 - tempDiff*4.0
	if tempScore < 0 {
		tempScore = 0
	}

	humDiff := math.Abs(humidity - 50.0)
	humScore := 100.0 - humDiff*2.0
	if humScore < 0 {
		humScore = 0
	}

	windScore := 100.0 - wind*5.0
	if windScore < 0 {
		windScore = 0
	}

	airComfort := float64(airScore)
	trafficComfort := 100.0 - float64(trafficScore)
	if trafficComfort < 0 {
		trafficComfort = 0
	}
	crimeComfort := 100.0 - float64(crimeScore)
	if crimeComfort < 0 {
		crimeComfort = 0
	}

	econScore := 50.0
	if gdp > 0 && population > 0 {
		gdpPerCap := gdp / float64(population)
		econScore = math.Log10(gdpPerCap+1.0) * 20.0
		if econScore < 0 {
			econScore = 0
		}
		if econScore > 100 {
			econScore = 100
		}
	}

	weights := map[string]float64{
		"temp":    0.22,
		"hum":     0.12,
		"wind":    0.06,
		"air":     0.25,
		"traffic": 0.15,
		"crime":   0.15,
		"econ":    0.05,
	}

	score := tempScore*weights["temp"] + humScore*weights["hum"] + windScore*weights["wind"] + airComfort*weights["air"] + trafficComfort*weights["traffic"] + crimeComfort*weights["crime"] + econScore*weights["econ"]

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return math.Round(score*10.0) / 10.0
}
