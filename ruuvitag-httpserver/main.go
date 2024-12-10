package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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
	TxPower                   int8    `json:"txPower"`
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

		err := writeToPostgres(m)
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

func writeToPostgres(m *Measurement) error {

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
			return err
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
		return fmt.Errorf("Unknown mac %s, skipping writing data to Postgresql", m.MAC)
	}

	createdAt := time.Now().Truncate(time.Minute)
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
		return err
	}

	return nil
}
