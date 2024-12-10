package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/joho/godotenv"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/rs/zerolog/log"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v2"
)

type Measurement struct {
	MAC                       string  `json:"mac"`
	Temperature               float64 `json:"temp"`
	Humidity                  float64 `json:"humidity"`
	Pressure                  uint32  `json:"pressure"`
	AccelerationX             int16   `json:"accelerationX"`
	AccelerationY             int16   `json:"accelerationY"`
	AccelerationZ             int16   `json:"accelerationZ"`
	Battery                   uint16  `json:"battery"`
	TxPower                   uint8   `json:"txPower"`
	MovementCounter           uint8   `json:"movementCounter"`
	MeasurementSequenceNumber uint16  `json:"measurementSequenceNumber"`
	Rssi                      int     `json:"rssi"`
}

const CONFIG_PATH = "config.yml"

var (
	configuration = map[string]string{}
	envFile       = map[string]string{}
	devices       = map[string]int{}
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

		writeToPostgres(m)

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

func writeToPostgres(m *Measurement) {

	log.Debug().Msg("Writing to Posgresql...")

	connUrl := envFile["POSTGRESQL_CONN_URL"]
	conn, err := pgx.Connect(context.Background(), connUrl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())

	if len(devices) == 0 {
		rows, err := conn.Query(context.Background(), "select id, mac, label from device")
		if err != nil {
			log.Error().Err(err).Msg("Failed to query all devices")
			return
		}

		for rows.Next() {
			var (
				id    int
				mac   string
				label string
			)
			rows.Scan(&id, &mac, &label)
			log.Debug().Msgf("id=%d, mac=%s, label=%s\n", id, mac, label)
			devices[strings.ToLower(mac)] = id
		}
	}

	deviceId, has := devices[strings.ToLower(m.MAC)]
	if !has {
		log.Warn().Msgf("Unknown mac %s, skipping writing data to Postgresql", m.MAC)
		return
	}

	createdAt := time.Now()
	createdAt = createdAt.Add(time.Duration(-1*createdAt.Second()) * time.Second)

	var measurementId int
	err = conn.QueryRow(context.Background(), "select id from measurement where device_id=$1 and created_at=$2", deviceId, createdAt).Scan(&measurementId)
	if err != nil {
		measurementId = -1
		log.Error().Err(err).Msgf("Failed to query device %d measurement at %v", deviceId, createdAt)
	}

	if measurementId == -1 {
		_, err = conn.Exec(context.Background(), "insert into measurement (device_id, created_at, temperature, humidity, pressure, acceleration_x, acceleration_y, acceleration_z, battery_voltage, tx_power, movement_counter, measurement_sequence_number, rssi) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)", deviceId, createdAt, m.Temperature, m.Humidity, m.Pressure, m.AccelerationX, m.AccelerationY, m.AccelerationZ, m.Battery, m.TxPower, m.MovementCounter, m.MeasurementSequenceNumber, m.Rssi)
	} else {
		_, err = conn.Exec(context.Background(), "update measurement set device_id=$1, created_at=$2, temperature=$3, humidity=$4, pressure=$5, acceleration_x=$6, acceleration_y=$7, acceleration_z=$8, battery_voltage=$9, tx_power=$10, movement_counter=$11, measurement_sequence_number=$12, rssi=$13) where id=$4", deviceId, createdAt, m.Temperature, m.Humidity, m.Pressure, m.AccelerationX, m.AccelerationY, m.AccelerationZ, m.Battery, m.TxPower, m.MovementCounter, m.MeasurementSequenceNumber, m.Rssi, measurementId)
	}

	if err != nil {
		log.Error().Err(err).Msgf("Failed to insert data for device %d", deviceId)
	}

}
