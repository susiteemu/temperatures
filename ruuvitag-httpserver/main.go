package main

import (
	"context"
	"os"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/joho/godotenv"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type Measurement struct {
	MAC         string  `json:"mac"`
	Temperature float64 `json:"temp"`
	Humidity    float64 `json:"humidity"`
	Pressure    uint32  `json:"pressure"`
	Battery     uint16  `json:"battery"`
}

const CONFIG_PATH = "config.yml"

var (
	configuration = map[string]string{}
	envFile       = map[string]string{}
)

func loadConfiguration() {
	file, err := os.ReadFile("config.yml")
	if err != nil {
		log.Error().Err(err).Msgf("Failed to read %s", CONFIG_PATH)
		panic(err)
	}

	err = yaml.Unmarshal(file, &configuration)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to unmarshal configuration")
		panic(err)
	}
	log.Info().Msgf("Loaded configuration: %v", configuration)

	envFile, err = godotenv.Read(".env")
	if err != nil {
		log.Error().Err(err).Msg("Failed to read from env file")
		panic(err)
	}

	log.Debug().Msgf("Read environment %v", envFile)
}

func main() {
	loadConfiguration()

	postMeasurement := func(c echo.Context) error {
		m := new(Measurement)
		if err := c.Bind(m); err != nil {
			log.Error().Err(err).Msgf("Failed to bind payload into measurement")
			return echo.NewHTTPError(400, "Invalid data")
		}
		log.Info().Msgf("Received new measurement: %v", m)

		deviceKey := strings.ToUpper(strings.ReplaceAll(m.MAC, ":", "_"))

		label, ok := configuration[deviceKey]
		if !ok {
			return echo.NewHTTPError(400, "Unknown MAC address")
		}

		err := writeToInfluxDB(m.Temperature, m.Humidity, m.Pressure, m.Battery, m.MAC, label)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to write to Influxdb")
			return echo.NewHTTPError(500, "Failed to write data")
		}

		return c.NoContent(200)
	}

	e := echo.New()
	e.Static("/static", "assets")
	e.Static("/css", "css")
	e.Use(middleware.Logger())
	e.POST("/measurements", postMeasurement)
	e.Logger.Fatal(e.Start(":1323"))

}

func writeToInfluxDB(t float64, h float64, p uint32, b uint16, mac string, label string) error {

	log.Debug().Msg("Writing to Influxdb...")

	url := envFile["INFLUXDB_URL"]
	token := envFile["INFLUXDB_TOKEN"]
	client := influxdb2.NewClient(url, token)

	org := envFile["INFLUXDB_ORG"]
	bucket := envFile["INFLUXDB_BUCKET"]
	writeAPI := client.WriteAPIBlocking(org, bucket)
	tags := map[string]string{
		"mac":       mac,
		"tag_label": label,
	}
	fields := map[string]interface{}{
		"temperature":    t,
		"humidity":       h,
		"pressure":       p,
		"batteryVoltage": b,
	}
	point := write.NewPoint("ruuvi_measurements", tags, fields, time.Now())

	err := writeAPI.WritePoint(context.Background(), point)
	return err
}
