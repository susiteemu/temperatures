package main

import (
	"fmt"
	"time"

	"github.com/Jeffail/gabs/v2"
	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type restyZeroLogger struct {
	logger *zerolog.Logger
}

func newLogger(zlogger *zerolog.Logger) *restyZeroLogger {
	return &restyZeroLogger{
		logger: zlogger,
	}
}
func (l *restyZeroLogger) Errorf(format string, v ...interface{}) {
	l.logger.Error().Msgf(format, v...)
}

func (l *restyZeroLogger) Warnf(format string, v ...interface{}) {
	l.logger.Warn().Msgf(format, v...)
}

func (l *restyZeroLogger) Debugf(format string, v ...interface{}) {
	l.logger.Debug().Msgf(format, v...)
}

func getWeather() (*Weather, error) {

	appId := envFile["OPENWEATHERMAP_API_KEY"]
	coordsLon := envFile["COORDS_LON"]
	coordsLat := envFile["COORDS_LAT"]

	url := fmt.Sprintf("https://api.openweathermap.org/data/3.0/onecall?lat=%s&lon=%s&exclude=minutely,daily&appid=%s&units=metric", coordsLat, coordsLon, appId)

	var client = resty.New().SetLogger(newLogger(&log.Logger))
	r := client.R()

	resp, err := r.Get(url)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query weather")
		return nil, err
	}
	if resp.IsError() {
		log.Error().Msgf("Got %d as response code", resp.StatusCode())
		return nil, err
	}
	body := resp.Body()

	jsonParsed, err := gabs.ParseJSON(body)
	if err != nil {
		return nil, err
	}

	currentWeather := jsonParsed.Path("current")
	currentWeatherIcon := currentWeather.Path("weather.0.icon").Data().(string)
	currentHourPrecipitation := getPrecipitation(currentWeather)

	weather := &Weather{
		Icon:          currentWeatherIcon,
		Precipitation: currentHourPrecipitation,
	}

	hourly := []Forecast{}
	for idx, child := range jsonParsed.S("hourly").Children() {

		if idx == 0 {
			// skip the first as it concerns current hour (we use current weather data for this)
			continue
		}
		if idx > 4 {
			// take only few first ones
			break
		}

		dt := int64(child.Path("dt").Data().(float64))
		dTime := time.Unix(0, dt*int64(time.Second))
		weatherIcon := child.Path("weather.0.icon").Data().(string)
		temp := child.Path("temp").Data().(float64)
		feelsLike := child.Path("feels_like").Data().(float64)
		pop := child.Path("pop").Data().(float64)

		precipitation := getPrecipitation(child)

		forecast := Forecast{
			Icon:          weatherIcon,
			Dt:            dTime,
			Temp:          temp,
			FeelsLike:     feelsLike,
			Precipitation: precipitation,
			Pop:           int(pop * 100),
		}

		hourly = append(hourly, forecast)
	}

	weather.Hourly = hourly

	return weather, nil
}

func getPrecipitation(container *gabs.Container) float64 {
	precipitation := 0.0
	if container.Exists("snow") {
		precipitation, _ = container.Path("snow.1h").Data().(float64)
	} else if container.Exists("rain") {
		precipitation, _ = container.Path("rain.1h").Data().(float64)
	}
	return precipitation
}
