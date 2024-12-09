package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-ble/ble"
	"github.com/go-ble/ble/linux"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	"github.com/peterhellberg/ruuvitag"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
)

const CONFIG_PATH = "config.yml"

var (
	configuration = map[string]string{}
	envFile       = map[string]string{}
	handledMacs   = []string{}
	macs          = []string{}
	devices       = map[string]int{}
)

func loadConfiguration() {

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)
	file, err := os.ReadFile(path.Join(exPath, "config.yml"))
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

	for k := range configuration {
		macs = append(macs, strings.ToUpper(strings.ReplaceAll(k, "_", ":")))
	}

	envFile, err = godotenv.Read(path.Join(exPath, ".env"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to read from env file")
		panic(err)
	}

	log.Debug().Msgf("Read environment %v", envFile)
}

func setup(ctx context.Context) context.Context {
	d, err := linux.NewDevice()
	if err != nil {
		panic(err)
	}
	ble.SetDefaultDevice(d)

	return ble.WithSigHandler(context.WithCancel(ctx))
}

func main() {
	log.Info().Msg("Loading configuration...")
	loadConfiguration()

	log.Info().Msg("Setting up...")
	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), 1*time.Minute))
	ctx = setup(ctx)

	log.Info().Msg("Scanning...")
	ble.Scan(ctx, false, handler, filter)
}

func handler(a ble.Advertisement) {
	log.Debug().Msgf("Handling %s", a.LocalName())

	deviceKey := strings.ToUpper(strings.ReplaceAll(a.Addr().String(), ":", "_"))

	label, ok := configuration[deviceKey]

	if !ok {
		log.Warn().Msgf("Got device with addr %s and converted device key %s that does not exist in configuration", a.Addr().String(), deviceKey)
		return
	}

	if ruuvitag.IsRAWv1(a.ManufacturerData()) {
		raw, err := ruuvitag.ParseRAWv1(a.ManufacturerData())
		if err != nil {
			log.Error().Err(err).Msgf("Failed to parse v1 data from device %s", a.Addr())
			return
		}
		handle(raw.Temperature, raw.Humidity, raw.Pressure, raw.Acceleration.X, raw.Acceleration.Y, raw.Acceleration.Z, raw.Battery, 0, 0., 0, a.RSSI(), a.Addr().String(), label)
	} else if ruuvitag.IsRAWv2(a.ManufacturerData()) {
		raw, err := ruuvitag.ParseRAWv2(a.ManufacturerData())
		if err != nil {
			log.Error().Err(err).Msgf("Failed to parse v2 data from device %s", a.Addr())
			return
		}
		log.Debug().Msgf("[%s] %s, RSSI: %3d: %+v\n", a.Addr(), label, a.RSSI(), raw)
		handle(raw.Temperature, raw.Humidity, raw.Pressure, raw.Acceleration.X, raw.Acceleration.Y, raw.Acceleration.Z, raw.Battery, raw.TXPower, raw.Movement, raw.Sequence, a.RSSI(), a.Addr().String(), label)
	} else {
		log.Error().Msgf("Got an advertisement that did not belong to any known Ruuvitag %s", a.Addr())
		return
	}
}

func filter(a ble.Advertisement) bool {
	return slices.Contains(macs, strings.ToUpper(a.Addr().String()))
}

func handle(t float64, h float64, p uint32, ax int16, ay int16, az int16,
	b uint16, tx int8, mv uint8, seq uint16, rssi int, mac string, label string) {

	if slices.Contains(handledMacs, mac) {
		log.Debug().Msgf("Device with mac %s already handled this round", mac)
		return
	}

	err := writeToInfluxDB(t, h, p, ax, ay, az, b, tx, mv, seq, rssi, mac, label)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to write data to Influxdb from device %s", mac)
	} else {
		log.Info().Msgf("Successfully wrote data to Influxdb from device %s", mac)
		handledMacs = append(handledMacs, mac)
	}

	writeToPostgres(t, h, p, ax, ay, az, b, tx, mv, seq, rssi, mac)
}

func writeToInfluxDB(t float64, h float64, p uint32, ax int16, ay int16, az int16,
	b uint16, tx int8, mv uint8, seq uint16, rssi int, mac string, label string) error {

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
		"temperature":               t,
		"humidity":                  h,
		"pressure":                  p,
		"accelerationX":             ax,
		"accelerationY":             ay,
		"accelerationZ":             az,
		"batteryVoltage":            b,
		"txPower":                   tx,
		"movementCounter":           mv,
		"measurementSequenceNumber": seq,
		"rssi":                      rssi,
	}
	point := write.NewPoint("ruuvi_measurements", tags, fields, time.Now())

	err := writeAPI.WritePoint(context.Background(), point)
	return err
}

func writeToPostgres(t float64, h float64, p uint32, ax int16, ay int16, az int16,
	b uint16, tx int8, mv uint8, seq uint16, rssi int, mac string) {

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

	deviceId, has := devices[strings.ToLower(mac)]
	if !has {
		log.Warn().Msgf("Unknown mac %s, skipping writing data to Postgresql", mac)
		return
	}

	createdAt := time.Now()
	_, err = conn.Exec(context.Background(), "insert into measurement (device_id, created_at, temperature, humidity, pressure, acceleration_x, acceleration_y, acceleration_z, battery_voltage, tx_power, movement_counter, measurement_sequence_number, rssi) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)", deviceId, createdAt, t, h, p, ax, ay, az, b, tx, mv, seq, rssi)

	if err != nil {
		log.Error().Err(err).Msgf("Failed to insert data for device %d", deviceId)
	}

}
