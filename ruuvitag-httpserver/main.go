package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"ruuvitag-httpserver/.gen/ruuvi/public/model"
	. "ruuvitag-httpserver/.gen/ruuvi/public/table"

	. "github.com/go-jet/jet/v2/postgres"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog/log"

	"gopkg.in/yaml.v2"

	_ "github.com/lib/pq"
)

type MeasurementJson struct {
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
	db            *sql.DB
	configuration = map[string]string{}
	envFile       = map[string]string{}
	devices       = map[string]int64{}
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

	var err error
	connectString := envFile["POSTGRESQL_CONN_URL"]
	db, err = sql.Open("postgres", connectString)
	if err != nil {
		panic(err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	defer db.Close()

	postMeasurement := func(c echo.Context) error {
		m := new(MeasurementJson)
		if err := c.Bind(m); err != nil {
			log.Error().Err(err).Msgf("Failed to bind payload into measurement")
			return echo.NewHTTPError(400, "Invalid data")
		}
		log.Info().Msgf("Received new measurement: %v", m)

		err := writeToPostgresWithJet(m)
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

func writeToPostgresWithJet(m *MeasurementJson) error {
	var err error

	if len(devices) == 0 {
		stmt := SELECT(Device.ID, Device.Mac).FROM(Device)
		var allDevices []struct {
			model.Device
		}
		stmt.Query(db, &allDevices)
		for _, device := range allDevices {
			devices[strings.ToLower(device.Mac)] = int64(device.ID)
		}
	}

	deviceId, has := devices[strings.ToLower(m.MAC)]
	if !has {
		log.Warn().Msgf("Unknown mac %s, skipping writing data to Postgresql", m.MAC)
		return fmt.Errorf("unknown mac %s, skipping writing data to Postgresql", m.MAC)
	}
	createdAt := time.Now().Truncate(time.Minute)
	var measurementId int64

	selectMeasurementStmt := SELECT(Measurement.ID).FROM(Measurement).WHERE(Measurement.DeviceID.EQ(Int(deviceId)).AND(Measurement.CreatedAt.EQ(TimestampzT(createdAt))))

	err = selectMeasurementStmt.Query(db, &measurementId)
	if err != nil {
		measurementId = -1
		log.Error().Err(err).Msgf("Failed to query device %d measurement at %v", deviceId, createdAt)
	}

	if measurementId == -1 {
		insertStmt := Measurement.
			INSERT(Measurement.DeviceID, Measurement.CreatedAt, Measurement.Temperature, Measurement.Humidity, Measurement.Pressure, Measurement.AccelerationX, Measurement.AccelerationY, Measurement.AccelerationZ, Measurement.BatteryVoltage, Measurement.TxPower, Measurement.MovementCounter, Measurement.MeasurementSequenceNumber, Measurement.Rssi).
			VALUES(deviceId, createdAt, m.Temperature, m.Humidity, m.Pressure, m.AccelerationX, m.AccelerationY, m.AccelerationZ, m.Battery, m.TxPower, m.MovementCounter, m.MeasurementSequenceNumber, m.Rssi)

		_, err = insertStmt.Exec(db)
	} else {
		updateStmt := Measurement.
			UPDATE(Measurement.DeviceID, Measurement.CreatedAt, Measurement.Temperature, Measurement.Humidity, Measurement.Pressure, Measurement.AccelerationX, Measurement.AccelerationY, Measurement.AccelerationZ, Measurement.BatteryVoltage, Measurement.TxPower, Measurement.MovementCounter, Measurement.MeasurementSequenceNumber, Measurement.Rssi).
			SET(deviceId, createdAt, m.Temperature, m.Humidity, m.Pressure, m.AccelerationX, m.AccelerationY, m.AccelerationZ, m.Battery, m.TxPower, m.MovementCounter, m.MeasurementSequenceNumber, m.Rssi).
			WHERE(Measurement.ID.EQ(Int(measurementId)))

		_, err = updateStmt.Exec(db)
	}

	if err != nil {
		log.Error().Err(err).Msgf("Failed to insert or update data for device %d", deviceId)
		return err
	}
	return nil
}
