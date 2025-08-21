package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"ruuvitag-httpserver/.gen/ruuvi/public/model"
	. "ruuvitag-httpserver/.gen/ruuvi/public/table"

	. "github.com/go-jet/jet/v2/postgres"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/rs/zerolog/log"

	_ "github.com/lib/pq"
)

type MeasurementJson struct {
	MAC                       string  `json:"mac"`
	Temperature               float64 `json:"temp"`
	Humidity                  float64 `json:"humidity"`
	Pressure                  int32   `json:"pressure"`
	AccelerationX             int32   `json:"accelerationX"`
	AccelerationY             int32   `json:"accelerationY"`
	AccelerationZ             int32   `json:"accelerationZ"`
	Battery                   int32   `json:"battery"`
	TxPower                   int32   `json:"txPower"`
	MovementCounter           int64   `json:"movementCounter"`
	MeasurementSequenceNumber int64   `json:"measurementSequenceNumber"`
	Rssi                      int32   `json:"rssi"`
}

var (
	db         *sql.DB
	mqttClient mqtt.Client
	envFile    = map[string]string{}
	devices    = map[string]int32{}
)

func loadConfiguration() {
	var err error
	envFile, err = godotenv.Read(".env")
	if err != nil {
		log.Error().Err(err).Msg("Failed to read from env file")
		panic(err)
	}
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

	opts := mqtt.NewClientOptions().
		AddBroker(envFile["MQTT_BROKER"]).
		SetClientID("ruuvitag-httpserver").
		SetUsername(envFile["MQTT_USER_NAME"]).
		SetPassword(envFile["MQTT_USER_PASSWORD"]).
		SetKeepAlive(2 * time.Second).
		SetPingTimeout(1 * time.Second)

	mqttClient = mqtt.NewClient(opts)
	if token := mqttClient.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal().Msgf("MQTT connection error: %v", token.Error())
	}

	postMeasurement := func(c echo.Context) error {
		m := new(MeasurementJson)
		if err := c.Bind(m); err != nil {
			log.Error().Err(err).Msgf("Failed to bind payload into measurement")
			return echo.NewHTTPError(400, "Invalid data")
		}
		log.Info().Msgf("Received new measurement: %v", m)

		err := storeMeasurement(m)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to write to data")
			return echo.NewHTTPError(500, "Failed to write data")
		}

		return c.NoContent(200)
	}

	postBinaryMeasurement := func(c echo.Context) error {
		binaryData, err := io.ReadAll(c.Request().Body)
		log.Info().Msgf("Binary data: %v", binaryData)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to read binary body")
			return echo.NewHTTPError(400, "Invalid data")
		}

		if len(binaryData) < 16 {
			log.Error().Err(err).Msgf("Binary body is wrong length: %d", len(binaryData))
			return echo.NewHTTPError(400, "Invalid data: wrong length")
		}
		// Check that data has expected manufacturer id and version
		// Mnufacturer: 0x1CA3
		// Version: 0x01
		if !bytes.HasPrefix(binaryData, []byte{0x1C, 0xA3, 0x01}) {
			log.Error().Msgf("Data prefix is unexpected. Full data: %v", binaryData)
			return echo.NewHTTPError(400, "Invalid data: prefix is unexpected")
		}

		rawTemperature := int16(binaryData[3])<<8 | int16(binaryData[4])
		temperature := 0.005 * float64(rawTemperature)

		rawHumidity := int16(binaryData[5])<<8 | int16(binaryData[6])
		humidity := 0.0025 * float64(rawHumidity)

		rawBattery := uint16(binaryData[7])<<3 | uint16(binaryData[8])>>5
		battery := rawBattery + 1600

		rawMac := binaryData[9:15]
		var hexParts []string
		for _, b := range rawMac {
			hexParts = append(hexParts, fmt.Sprintf("%02x", b))
		}
		macString := strings.Join(hexParts, ":")

		m := new(MeasurementJson)
		m.Temperature = temperature
		m.Humidity = humidity
		m.Battery = int32(battery)
		m.MAC = macString

		log.Info().Msgf("Received new measurement: %v", m)

		err = storeMeasurement(m)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to write data")
			return echo.NewHTTPError(500, "Failed to write data")
		}

		return c.NoContent(200)
	}

	e := echo.New()
	e.Static("/static", "assets")
	e.Static("/css", "css")
	e.Use(middleware.Logger())
	e.POST("/measurements", postMeasurement)
	e.POST("/v2/measurements", postBinaryMeasurement)
	e.Logger.Fatal(e.Start(":1323"))
}

func storeMeasurement(m *MeasurementJson) error {
	var err error

	if len(devices) == 0 {
		stmt := SELECT(Device.ID, Device.Label, Device.Mac).FROM(Device)
		var allDevices []struct {
			model.Device
		}
		err = stmt.Query(db, &allDevices)
		if err != nil {
			log.Error().Err(err).Msg("Failed to select all devices")
			return err
		}
		for _, device := range allDevices {
			devices[strings.ToLower(device.Mac)] = int32(device.ID)

			mqttSensorMac := strings.ReplaceAll(strings.ToLower(device.Mac), ":", "_")
			discoveryTopic := fmt.Sprintf("homeassistant/sensor/%s_temperature/config", mqttSensorMac)
			stateTopic := fmt.Sprintf("home/temperature/%s", mqttSensorMac)

			payload := map[string]any{
				"name":                device.Label,
				"unique_id":           fmt.Sprintf("%s_temperature", mqttSensorMac),
				"state_topic":         stateTopic,
				"unit_of_measurement": "°C",
				"device_class":        "temperature",
				"value_template":      "{{ value_json.temp }}",
			}

			data, _ := json.Marshal(payload)
			token := mqttClient.Publish(discoveryTopic, 0, true, data)
			token.WaitTimeout(500 * time.Millisecond)
			log.Printf("Published discovery for %s", mqttSensorMac)

		}
	}

	deviceId, has := devices[strings.ToLower(m.MAC)]
	if !has {
		log.Warn().Msgf("Unknown mac %s, skipping writing data to Postgresql", m.MAC)
		return fmt.Errorf("unknown mac %s, skipping writing data to Postgresql", m.MAC)
	}
	createdAt := time.Now().Truncate(time.Minute)
	var measurement model.Measurement

	selectMeasurementStmt := SELECT(Measurement.AllColumns).FROM(Measurement).WHERE(Measurement.DeviceID.EQ(Int32(deviceId)).AND(Measurement.CreatedAt.EQ(TimestampzT(createdAt))))

	err = selectMeasurementStmt.Query(db, &measurement)
	if err != nil {
		measurement.ID = -1
		measurement.CreatedAt = createdAt
	}

	measurement.DeviceID = int32(deviceId)
	measurement.Temperature = &m.Temperature
	measurement.Humidity = &m.Humidity
	measurement.Pressure = &m.Pressure
	measurement.AccelerationX = &m.AccelerationX
	measurement.AccelerationY = &m.AccelerationY
	measurement.AccelerationZ = &m.AccelerationZ
	measurement.BatteryVoltage = &m.Battery
	measurement.TxPower = &m.TxPower
	measurement.MovementCounter = &m.MovementCounter
	measurement.MeasurementSequenceNumber = &m.MeasurementSequenceNumber
	measurement.Rssi = &m.Rssi

	if measurement.ID == -1 {
		insertStmt := Measurement.
			INSERT(Measurement.MutableColumns).
			MODEL(measurement)

		_, err = insertStmt.Exec(db)

		selectRoomStmt := SELECT(Device.AllColumns).FROM(Device).WHERE(Device.ID.EQ(Int32(deviceId)))
		var device model.Device
		err := selectRoomStmt.Query(db, &device)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to get device label for id %d", deviceId)
			return err
		}
		room := device.Label

		mqttSensorMac := strings.ReplaceAll(strings.ToLower(device.Mac), ":", "_")
		stateTopic := fmt.Sprintf("home/temperature/%s", mqttSensorMac)
		payload := map[string]any{
			"room": room,
			"temp": measurement.Temperature,
		}
		data, _ := json.Marshal(payload)

		token := mqttClient.Publish(stateTopic, 0, false, data)
		published := token.WaitTimeout(500 * time.Millisecond)
		if published {
			log.Info().Msgf("Published state %.2f°C for %s", *measurement.Temperature, room)
		} else {
			log.Error().Msgf("Failed to publish state for %s", m.MAC)
		}
	}

	if err != nil {
		log.Error().Err(err).Msgf("Failed to write data for device %d", deviceId)
		return err
	}
	return nil
}
